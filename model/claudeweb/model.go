package claudeweb

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"strings"
	"sync"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/model"
)

// Model implements model.LLM using the claude.ai web API as backend.
type Model struct {
	client    *Client
	modelName string
	effort    string // "high", "medium", "low"

	mu                sync.Mutex
	convID            string
	lastAssistantUUID string
}

// NewModel creates a Model backed by the claude.ai web API.
func NewModel(client *Client, modelName string, effort string) *Model {
	if effort == "" {
		effort = "medium"
	}
	return &Model{
		client:    client,
		modelName: modelName,
		effort:    effort,
	}
}

func (m *Model) Name() string { return m.modelName }

// GenerateContent implements model.LLM.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if req == nil {
		return singleError(fmt.Errorf("claudeweb: nil request"))
	}

	webReq, isToolResult, err := m.buildRequest(req)
	if err != nil {
		return singleError(fmt.Errorf("claudeweb: build request: %w", err))
	}

	m.mu.Lock()
	convID := m.convID
	if convID == "" {
		convID = generateUUID()
		m.convID = convID
		webReq.CreateConversationParams = &CreateConversationParams{
			Name:                          "",
			Model:                         m.modelName,
			IncludeConversationPreferences: true,
			IsTemporary:                   true,
		}
	}
	if isToolResult {
		webReq.ParentMessageUUID = m.lastAssistantUUID
	}
	m.mu.Unlock()

	log.Printf("claudeweb: sending request convID=%s prompt=%q isToolResult=%v tools=%d",
		convID, truncate(webReq.Prompt, 50), isToolResult, len(webReq.Tools))

	return func(yield func(*model.LLMResponse, error) bool) {
		body, err := m.client.Completion(convID, webReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		events := ParseSSEStream(body)

		var blocks []contentBlockState
		var stopReason string

		for event := range events {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			default:
			}

			switch event.Event {
			case "ping", "conversation_ready":
				continue

			case "message_start":
				var ev MessageStartEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					log.Printf("claudeweb: parse message_start: %v", err)
					continue
				}
				m.mu.Lock()
				m.lastAssistantUUID = ev.Message.UUID
				m.mu.Unlock()

			case "content_block_start":
				var ev ContentBlockStartEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					log.Printf("claudeweb: parse content_block_start: %v", err)
					continue
				}
				for len(blocks) <= ev.Index {
					blocks = append(blocks, contentBlockState{})
				}
				blocks[ev.Index] = contentBlockState{
					blockType: ev.ContentBlock.Type,
					id:        ev.ContentBlock.ID,
					name:      ev.ContentBlock.Name,
				}

			case "content_block_delta":
				var ev ContentBlockDeltaEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					log.Printf("claudeweb: parse content_block_delta: %v", err)
					continue
				}
				if ev.Index >= len(blocks) {
					continue
				}
				block := &blocks[ev.Index]

				switch ev.Delta.Type {
				case "text_delta":
					block.text.WriteString(ev.Delta.Text)
					// Yield partial for streaming display
					if stream {
						resp := &model.LLMResponse{
							Content: &genai.Content{
								Role: "model",
								Parts: []*genai.Part{
									{Text: ev.Delta.Text},
								},
							},
							Partial: true,
						}
						if !yield(resp, nil) {
							return
						}
					}
				case "input_json_delta":
					block.inputJSON.WriteString(ev.Delta.PartialJSON)
				}

			case "content_block_stop":
				// done

			case "message_delta":
				var ev MessageDeltaEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					log.Printf("claudeweb: parse message_delta: %v", err)
					continue
				}
				if ev.Delta.StopReason != nil {
					stopReason = *ev.Delta.StopReason
				}

			case "message_stop":
				resp := buildFinalResponse(blocks, stopReason)
				resp.TurnComplete = true
				yield(resp, nil)
				return

			case "message_limit":
				log.Printf("claudeweb: message_limit event received")

			case "error":
				yield(nil, fmt.Errorf("claudeweb: server error: %s", string(event.Data)))
				return

			default:
				log.Printf("claudeweb: unknown event: %s", event.Event)
			}
		}

		if len(blocks) > 0 {
			resp := buildFinalResponse(blocks, stopReason)
			resp.TurnComplete = true
			yield(resp, nil)
		}
	}
}

// buildRequest converts an adk-go LLMRequest into a web API CompletionRequest.
func (m *Model) buildRequest(req *model.LLMRequest) (*CompletionRequest, bool, error) {
	webReq := &CompletionRequest{
		Model:         m.modelName,
		Timezone:      "Asia/Shanghai",
		Locale:        "en-US",
		Effort:        m.effort,
		ThinkingMode:  "off",
		RenderingMode: "messages",
		Attachments:   []json.RawMessage{},
		Files:         []json.RawMessage{},
		SyncSources:   []json.RawMessage{},
		Tools:         convertTools(req.Config),
	}

	if len(req.Contents) == 0 {
		return nil, false, fmt.Errorf("no contents in request")
	}

	lastContent := req.Contents[len(req.Contents)-1]

	// Check if the last message contains tool results (function responses)
	var toolResults []ToolResult
	for _, part := range lastContent.Parts {
		if part.FunctionResponse != nil {
			resultJSON, _ := json.Marshal(part.FunctionResponse.Response)
			toolResults = append(toolResults, ToolResult{
				ToolUseID: part.FunctionResponse.ID,
				Content:   string(resultJSON),
			})
		}
	}

	if len(toolResults) > 0 {
		webReq.Prompt = ""
		webReq.ToolResults = toolResults
		return webReq, true, nil
	}

	// Normal user message: extract text from the last user content
	var prompt strings.Builder
	for _, part := range lastContent.Parts {
		if part.Text != "" {
			prompt.WriteString(part.Text)
		}
	}
	webReq.Prompt = prompt.String()

	return webReq, false, nil
}

// convertTools converts genai tool config to the web API tool format.
func convertTools(config *genai.GenerateContentConfig) []WebTool {
	if config == nil {
		return []WebTool{}
	}

	var webTools []WebTool
	for _, t := range config.Tools {
		for _, fd := range t.FunctionDeclarations {
			schema := buildInputSchema(fd.Parameters)
			webTools = append(webTools, WebTool{
				Name:        fd.Name,
				Description: fd.Description,
				InputSchema: schema,
			})
		}
	}
	if webTools == nil {
		return []WebTool{}
	}
	return webTools
}

// buildInputSchema converts a genai.Schema to a JSON Schema object
// that the web API accepts.
func buildInputSchema(s *genai.Schema) json.RawMessage {
	if s == nil {
		return json.RawMessage(`{"type":"object","properties":{}}`)
	}

	schema := map[string]any{
		"type": strings.ToLower(string(s.Type)),
	}

	if len(s.Properties) > 0 {
		props := map[string]any{}
		for name, prop := range s.Properties {
			p := map[string]any{
				"type": strings.ToLower(string(prop.Type)),
			}
			if prop.Description != "" {
				p["description"] = prop.Description
			}
			if len(prop.Enum) > 0 {
				p["enum"] = prop.Enum
			}
			props[name] = p
		}
		schema["properties"] = props
	} else {
		schema["properties"] = map[string]any{}
	}

	if len(s.Required) > 0 {
		schema["required"] = s.Required
	}

	data, _ := json.Marshal(schema)
	return data
}

type contentBlockState struct {
	blockType string
	id        string
	name      string
	text      strings.Builder
	inputJSON strings.Builder
}

func buildFinalResponse(blocks []contentBlockState, stopReason string) *model.LLMResponse {
	content := &genai.Content{
		Role: "model",
	}

	for _, block := range blocks {
		switch block.blockType {
		case "text":
			if block.text.Len() > 0 {
				content.Parts = append(content.Parts, &genai.Part{
					Text: block.text.String(),
				})
			}
		case "tool_use":
			args := map[string]any{}
			if block.inputJSON.Len() > 0 {
				_ = json.Unmarshal([]byte(block.inputJSON.String()), &args)
			}
			content.Parts = append(content.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   block.id,
					Name: block.name,
					Args: args,
				},
			})
		}
	}

	resp := &model.LLMResponse{Content: content}

	switch stopReason {
	case "end_turn":
		resp.FinishReason = genai.FinishReasonStop
	case "tool_use":
		resp.FinishReason = genai.FinishReasonStop
	case "max_tokens":
		resp.FinishReason = genai.FinishReasonMaxTokens
	}

	return resp
}

func singleError(err error) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, err)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
