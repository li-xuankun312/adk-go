// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package llminternal

import (
	"encoding/json"
	"fmt"
	"iter"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/internal/utils"
	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/authconsent"
)

// RequestCredentialRequestProcessor resumes tool calls paused for interactive
// (3-legged) OAuth consent. It is the credential twin of
// [RequestConfirmationRequestProcessor] and the Go port of adk-python's
// auth_preprocessor: when the latest user turn carries adk_request_credential
// function responses, it matches each back to the paused tool call and re-runs
// that tool with the consent response threaded in.
func RequestCredentialRequestProcessor(ctx agent.InvocationContext, req *model.LLMRequest, f *Flow) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		if asLLMAgent(ctx.Agent()) == nil {
			return // Matching the confirmation processor: no error is yielded.
		}

		toolsmap := make(map[string]tool.Tool, len(f.Tools))
		for _, t := range f.Tools {
			toolsmap[t.Name()] = t
		}

		var events []*session.Event
		if ctx.Session() != nil {
			for e := range ctx.Session().Events().All() {
				events = append(events, e)
			}
		}

		// Collect the adk_request_credential responses from the latest user turn,
		// keyed by the id of the consent call each answers.
		consentResponses := make(map[string]authconsent.AuthConfig)
		consentEventIndex := -1
		for k := len(events) - 1; k >= 0; k-- {
			event := events[k]
			if event.Author != "user" {
				continue
			}
			responses := utils.FunctionResponses(event.Content)
			if len(responses) == 0 {
				return
			}
			for _, funcResp := range responses {
				if funcResp.Name != authconsent.FunctionCallName {
					continue
				}
				cfg, err := decodeConsentResponse(funcResp, event.ID)
				if err != nil {
					yield(nil, err)
					return
				}
				consentResponses[funcResp.ID] = cfg
			}
			consentEventIndex = k
			break
		}

		if len(consentResponses) == 0 {
			return
		}

		agentName := ctx.Agent().Name()

		// Resolve each consent call to the tool call it paused, reading
		// functionCallId off the agent-authored request rather than off the
		// client's reply. The same reasoning as the confirmation processor's
		// tampering check applies, one step earlier: the user's reply carries only
		// the consent call's id, so the binding from consent to tool call comes
		// entirely from an event in the session. Honour that binding only from an
		// event this agent authored, and refuse a consent id that two events bind
		// to different tool calls — a single consent cannot be known to cover both,
		// so fail closed and resume neither.
		resumeTarget := make(map[string]string, len(consentResponses))
		credentialKey := make(map[string]string, len(consentResponses))
		conflicting := make(map[string]bool)
		for _, event := range events {
			if event.Author != agentName {
				continue
			}
			for _, functionCall := range utils.FunctionCalls(event.Content) {
				if functionCall.Name != authconsent.FunctionCallName {
					continue
				}
				if _, ok := consentResponses[functionCall.ID]; !ok {
					continue
				}
				targetID, ok := pausedCallID(functionCall)
				if !ok {
					continue
				}
				if prev, seen := resumeTarget[functionCall.ID]; seen {
					if prev != targetID {
						conflicting[functionCall.ID] = true
					}
					continue
				}
				resumeTarget[functionCall.ID] = targetID
				credentialKey[functionCall.ID] = requestedCredentialKey(functionCall)
			}
		}

		// Refuse the mirror ambiguity too: two consent calls answered in the same
		// turn that both name one tool call. Ranging the map would pick a winner
		// by Go's randomized iteration order, so the tool would resume under one
		// credential key or the other at random. Neither consent is known to
		// cover the other, so resume under neither.
		targetCount := make(map[string]int, len(resumeTarget))
		for consentID, targetID := range resumeTarget {
			if !conflicting[consentID] {
				targetCount[targetID]++
			}
		}

		// Key the consent responses by the tool call to resume. The credential key
		// comes from the agent's own request, not from the client's reply, so a
		// client cannot point the resumed tool at another credential.
		authResponses := make(map[string]*authconsent.AuthConfig, len(resumeTarget))
		for consentID, targetID := range resumeTarget {
			if conflicting[consentID] || targetCount[targetID] > 1 {
				continue
			}
			cfg := consentResponses[consentID]
			cfg.CredentialKey = credentialKey[consentID]
			authResponses[targetID] = &cfg
		}
		if len(authResponses) == 0 {
			return
		}

		for k := len(events) - 2; k >= 0; k-- {
			event := events[k]
			// Only this agent's own tool calls may be resumed. Function call parts
			// reach the session from elsewhere too, notably an A2A peer response
			// converted into a model-role event, and resuming one of those would let
			// the peer choose which local tool runs and with what arguments — behind
			// a consent the user granted for something else.
			if event.Author != agentName {
				continue
			}
			calls := utils.FunctionCalls(event.Content)
			if len(calls) == 0 {
				continue
			}

			// Iterate calls (an ordered slice) rather than the authResponses map, so
			// the re-dispatched parts, the merged response and the resulting
			// StateDelta last-writer-wins merge are deterministic across runs.
			toResume := make(map[string]*authconsent.AuthConfig, len(authResponses))
			var resumeOrder []string
			for _, functionCall := range calls {
				cfg, ok := authResponses[functionCall.ID]
				if !ok {
					continue
				}
				if _, seen := toResume[functionCall.ID]; seen {
					continue
				}
				resumeOrder = append(resumeOrder, functionCall.ID)
				toResume[functionCall.ID] = cfg
			}
			if len(toResume) == 0 {
				continue
			}

			// Drop tool calls already answered after the consent arrived, so a
			// re-entered processor does not run the tool a second time.
			for j := len(events) - 1; j > consentEventIndex; j-- {
				for _, resp := range utils.FunctionResponses(events[j].Content) {
					delete(toResume, resp.ID)
				}
				if len(toResume) == 0 {
					break
				}
			}
			if len(toResume) == 0 {
				continue
			}

			parts := make([]*genai.Part, 0, len(toResume))
			for _, callID := range resumeOrder {
				if _, ok := toResume[callID]; !ok {
					continue
				}
				for _, functionCall := range calls {
					if functionCall.ID == callID {
						parts = append(parts, &genai.Part{FunctionCall: functionCall})
						break
					}
				}
			}

			ev, err := f.handleFunctionCalls(ctx, toolsmap, &model.LLMResponse{
				Content: &genai.Content{Parts: parts, Role: genai.RoleUser},
			}, nil, toResume, nil)
			yield(ev, err)
			// One consent turn resumes one batch of tool calls. adk-python returns
			// here too; continuing would re-dispatch any call whose id also appears
			// in an earlier event, running its tool twice.
			return
		}
	}
}

// pausedCallID returns the id of the tool call an adk_request_credential call is
// asking consent for. It is the "functionCallId" argument adk-python's
// AuthToolArguments carries.
func pausedCallID(functionCall *genai.FunctionCall) (string, bool) {
	if functionCall == nil || functionCall.Args == nil {
		return "", false
	}
	id, ok := functionCall.Args[authconsent.FunctionCallIDArg].(string)
	if !ok || id == "" {
		return "", false
	}
	return id, true
}

// requestedCredentialKey returns the credential key the agent put in the consent
// request, or "" when the request carried none. The request is re-encoded
// through JSON because a call read back from a session store holds its args as
// plain maps, while one still in memory holds the original struct.
func requestedCredentialKey(functionCall *genai.FunctionCall) string {
	if functionCall == nil || functionCall.Args == nil {
		return ""
	}
	raw, ok := functionCall.Args[authconsent.AuthConfigArg]
	if !ok {
		return ""
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	var cfg authconsent.AuthConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return ""
	}
	return cfg.CredentialKey
}

// decodeConsentResponse extracts an [authconsent.AuthConfig] from a client's
// function response, accepting two encodings: a plain object, which is what the
// ADK web client sends for consent, and the payload wrapped in a single
// "response" JSON string, which is what it sends for confirmation. Both are
// accepted because the confirmation processor accepts both and a client is free
// to use either.
func decodeConsentResponse(funcResp *genai.FunctionResponse, eventID string) (authconsent.AuthConfig, error) {
	var cfg authconsent.AuthConfig
	if funcResp.Response == nil {
		return cfg, nil
	}
	if raw, ok := funcResp.Response["response"]; ok && len(funcResp.Response) == 1 {
		s, ok := raw.(string)
		if !ok {
			return cfg, fmt.Errorf("credential response for event id %q: 'response' key is not a string", eventID)
		}
		if err := json.Unmarshal([]byte(s), &cfg); err != nil {
			return cfg, fmt.Errorf("credential response for event id %q: unmarshal 'response': %w", eventID, err)
		}
		return cfg, nil
	}
	b, err := json.Marshal(funcResp.Response)
	if err != nil {
		return cfg, fmt.Errorf("credential response for event id %q: marshal: %w", eventID, err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("credential response for event id %q: unmarshal: %w", eventID, err)
	}
	return cfg, nil
}
