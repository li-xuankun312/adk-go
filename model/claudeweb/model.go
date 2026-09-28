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
	Shadow    *ShadowExecutor // if set, mirrors remote tool calls locally

	mu             sync.Mutex
	convID         string
	lastSentPrompt string
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

// ResetConversation clears conversation state so next message starts fresh.
func (m *Model) ResetConversation() {
	m.mu.Lock()
	m.convID = ""
	m.lastSentPrompt = ""
	m.mu.Unlock()
}

// ConvID returns the current conversation UUID (for status display).
func (m *Model) ConvID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.convID == "" {
		return "(none)"
	}
	return m.convID[:8]
}

// toolBlock tracks a tool_use content block being streamed
type toolBlock struct {
	name      string
	inputJSON strings.Builder
}

func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if req == nil {
		return emptyResponse()
	}

	prompt := m.extractPrompt(req)
	if prompt == "" {
		return emptyResponse()
	}

	m.mu.Lock()
	if prompt == m.lastSentPrompt {
		m.mu.Unlock()
		log.Printf("claudeweb: skip (duplicate)")
		return emptyResponse()
	}
	m.lastSentPrompt = prompt

	convID := m.convID
	isNewConv := convID == ""
	if isNewConv {
		convID = generateUUID()
		m.convID = convID
	}
	m.mu.Unlock()

	webReq := &CompletionRequest{
		Prompt:        prompt,
		Model:         m.modelName,
		Timezone:      "Asia/Shanghai",
		Locale:        "en-US",
		Effort:        m.effort,
		ThinkingMode:  "off",
		RenderingMode: "messages",
		Attachments:   []json.RawMessage{},
		Files:         []json.RawMessage{},
		SyncSources:   []json.RawMessage{},
		Tools:         []WebTool{},
	}
	if isNewConv {
		webReq.CreateConversationParams = &CreateConversationParams{
			Name:                          "",
			Model:                         m.modelName,
			IncludeConversationPreferences: true,
			IsTemporary:                   true,
		}
	}

	log.Printf("claudeweb: → %s prompt=%q", convID[:8], truncate(prompt, 60))

	return func(yield func(*model.LLMResponse, error) bool) {
		body, err := m.client.Completion(convID, webReq)
		if err != nil {
			m.mu.Lock()
			m.convID = ""
			m.lastSentPrompt = ""
			m.mu.Unlock()
			yield(&model.LLMResponse{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: fmt.Sprintf("[API Error] %v", err)}},
				},
				TurnComplete: true,
				FinishReason: genai.FinishReasonStop,
			}, nil)
			return
		}
		defer body.Close()

		events := ParseSSEStream(body)
		var textBuf strings.Builder
		var shadowResults []string

		// Track tool_use blocks by index
		toolBlocks := map[int]*toolBlock{}

		for event := range events {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			default:
			}

			switch event.Event {
			case "ping", "conversation_ready", "message_limit", "message_delta":
				continue

			case "message_start":
				var ev MessageStartEvent
				if err := json.Unmarshal(event.Data, &ev); err == nil {
					m.mu.Lock()
					m.mu.Unlock()
				}

			case "content_block_start":
				var ev ContentBlockStartEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					continue
				}
				if ev.ContentBlock.Type == "tool_use" {
					toolBlocks[ev.Index] = &toolBlock{
						name: ev.ContentBlock.Name,
					}
					log.Printf("[remote] tool_use start: %s", ev.ContentBlock.Name)
				}

			case "content_block_delta":
				var ev ContentBlockDeltaEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					continue
				}

				switch ev.Delta.Type {
				case "text_delta":
					textBuf.WriteString(ev.Delta.Text)
					if stream {
						if !yield(&model.LLMResponse{
							Content: &genai.Content{
								Role:  "model",
								Parts: []*genai.Part{{Text: ev.Delta.Text}},
							},
							Partial: true,
						}, nil) {
							return
						}
					}
				case "input_json_delta":
					if tb, ok := toolBlocks[ev.Index]; ok {
						tb.inputJSON.WriteString(ev.Delta.PartialJSON)
					}
				}

			case "content_block_stop":
				var ev ContentBlockStopEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					continue
				}
				// Tool block finished → shadow execute!
				if tb, ok := toolBlocks[ev.Index]; ok {
					if m.Shadow != nil {
						result := m.Shadow.Execute(tb.name, tb.inputJSON.String())
						if result != "" {
							shadowResults = append(shadowResults, result)
						}
					}
					delete(toolBlocks, ev.Index)
				}

			case "message_stop":
				text := textBuf.String()
				// Append shadow execution results
				if len(shadowResults) > 0 {
					text += "\n\n━━━ Local Shadow Execution ━━━\n"
					for _, r := range shadowResults {
						text += r + "\n"
					}
				}
				if text == "" {
					text = "(empty response)"
				}
				yield(&model.LLMResponse{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{Text: text}},
					},
					TurnComplete: true,
					FinishReason: genai.FinishReasonStop,
				}, nil)
				return

			case "error":
				m.mu.Lock()
				m.convID = ""
				m.lastSentPrompt = ""
				m.mu.Unlock()
				yield(&model.LLMResponse{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{Text: fmt.Sprintf("[Error] %s", string(event.Data))}},
					},
					TurnComplete: true,
					FinishReason: genai.FinishReasonStop,
				}, nil)
				return
			}
		}

		if textBuf.Len() > 0 {
			text := textBuf.String()
			if len(shadowResults) > 0 {
				text += "\n\n━━━ Local Shadow Execution ━━━\n"
				for _, r := range shadowResults {
					text += r + "\n"
				}
			}
			yield(&model.LLMResponse{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: text}},
				},
				TurnComplete: true,
				FinishReason: genai.FinishReasonStop,
			}, nil)
		}
	}
}

func (m *Model) extractPrompt(req *model.LLMRequest) string {
	if len(req.Contents) == 0 {
		return ""
	}
	last := req.Contents[len(req.Contents)-1]
	if last.Role != "user" {
		return ""
	}
	for _, part := range last.Parts {
		if part.FunctionResponse != nil {
			return ""
		}
		if part.Text != "" {
			if strings.HasPrefix(part.Text, "Continue processing previous") {
				return ""
			}
			return part.Text
		}
	}
	return ""
}

func emptyResponse() iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: ""}},
			},
			TurnComplete: true,
			FinishReason: genai.FinishReasonStop,
		}, nil)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
