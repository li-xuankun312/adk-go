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

type Model struct {
	client    *Client
	modelName string
	effort    string

	mu                sync.Mutex
	convID            string
	lastAssistantUUID string
}

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

func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if req == nil {
		return singleError(fmt.Errorf("claudeweb: nil request"))
	}

	webReq, err := m.buildRequest(req)
	if err != nil {
		return singleError(fmt.Errorf("claudeweb: build request: %w", err))
	}
	// Skip sending if prompt is empty and it's not the first turn
	// (this happens when the runner tries to send tool results for
	// built-in tools that executed on the remote side)
	if webReq.Prompt == "" && webReq.CreateConversationParams == nil {
		log.Printf("claudeweb: skipping empty prompt (likely internal tool result round-trip)")
		return singleYield(&model.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: ""}},
			},
			TurnComplete: true,
			FinishReason: genai.FinishReasonStop,
		})
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
	m.mu.Unlock()

	log.Printf("claudeweb: request convID=%s prompt=%q", convID, truncate(webReq.Prompt, 80))

	return func(yield func(*model.LLMResponse, error) bool) {
		body, err := m.client.Completion(convID, webReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		events := ParseSSEStream(body)
		var textBuf strings.Builder

		for event := range events {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			default:
			}

			switch event.Event {
			case "ping", "conversation_ready", "message_limit":
				continue

			case "message_start":
				var ev MessageStartEvent
				if err := json.Unmarshal(event.Data, &ev); err == nil {
					m.mu.Lock()
					m.lastAssistantUUID = ev.Message.UUID
					m.mu.Unlock()
				}

			case "content_block_start":
				// We only care about text blocks; tool_use blocks
				// from built-in tools are ignored (they execute remotely)

			case "content_block_delta":
				var ev ContentBlockDeltaEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					continue
				}
				if ev.Delta.Type == "text_delta" {
					textBuf.WriteString(ev.Delta.Text)
					if stream {
						resp := &model.LLMResponse{
							Content: &genai.Content{
								Role:  "model",
								Parts: []*genai.Part{{Text: ev.Delta.Text}},
							},
							Partial: true,
						}
						if !yield(resp, nil) {
							return
						}
					}
				}
				// input_json_delta from tool_use blocks: silently skip

			case "content_block_stop":
				continue

			case "message_delta":
				// stop reason received

			case "message_stop":
				text := textBuf.String()
				if text == "" {
					text = "(no text response)"
				}
				resp := &model.LLMResponse{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{Text: text}},
					},
					TurnComplete: true,
					FinishReason: genai.FinishReasonStop,
				}
				yield(resp, nil)
				return

			case "error":
				yield(nil, fmt.Errorf("claudeweb: server error: %s", string(event.Data)))
				return
			}
		}

		// Stream ended without message_stop
		if textBuf.Len() > 0 {
			resp := &model.LLMResponse{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: textBuf.String()}},
				},
				TurnComplete: true,
				FinishReason: genai.FinishReasonStop,
			}
			yield(resp, nil)
		}
	}
}

func (m *Model) buildRequest(req *model.LLMRequest) (*CompletionRequest, error) {
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
		Tools:         []WebTool{}, // empty: let remote Claude use its own tools
	}

	if len(req.Contents) == 0 {
		return nil, fmt.Errorf("no contents in request")
	}

	// Walk contents backward to find the last real user text.
	// Skip synthetic "Continue processing" messages and FunctionResponse.
	var prompt string
	for i := len(req.Contents) - 1; i >= 0; i-- {
		c := req.Contents[i]
		if c.Role != "user" {
			continue
		}
		for _, part := range c.Parts {
			// Skip function responses (tool results from runner)
			if part.FunctionResponse != nil {
				continue
			}
			if part.Text != "" && part.Text != "Continue processing previous requests as instructed. Exit or provide a summary if no more outputs are needed." {
				prompt = part.Text
				break
			}
		}
		if prompt != "" {
			break
		}
	}

	webReq.Prompt = prompt
	return webReq, nil
}

func singleError(err error) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, err)
	}
}

func singleYield(resp *model.LLMResponse) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(resp, nil)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
