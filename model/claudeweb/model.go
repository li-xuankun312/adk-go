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
	lastSentPrompt    string // prevent re-sending the same prompt
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
		return emptyResponse()
	}

	prompt := m.extractPrompt(req)

	// Nothing to send: empty input, synthetic runner message, or duplicate
	if prompt == "" {
		log.Printf("claudeweb: skip (empty/synthetic prompt)")
		return emptyResponse()
	}

	m.mu.Lock()
	if prompt == m.lastSentPrompt {
		m.mu.Unlock()
		log.Printf("claudeweb: skip (duplicate prompt)")
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

	log.Printf("claudeweb: → convID=%s prompt=%q", convID[:8], truncate(prompt, 60))

	return func(yield func(*model.LLMResponse, error) bool) {
		body, err := m.client.Completion(convID, webReq)
		if err != nil {
			// On error, reset state so next real input creates a fresh conversation
			m.mu.Lock()
			m.convID = ""
			m.lastSentPrompt = ""
			m.mu.Unlock()
			log.Printf("claudeweb: API error: %v", err)
			// Return the error as text instead of an error, so the runner
			// does NOT retry. The runner retries on error; it stops on text.
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

		for event := range events {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			default:
			}

			switch event.Event {
			case "ping", "conversation_ready", "message_limit",
				"content_block_start", "content_block_stop", "message_delta":
				continue

			case "message_start":
				var ev MessageStartEvent
				if err := json.Unmarshal(event.Data, &ev); err == nil {
					m.mu.Lock()
					m.lastAssistantUUID = ev.Message.UUID
					m.mu.Unlock()
				}

			case "content_block_delta":
				var ev ContentBlockDeltaEvent
				if err := json.Unmarshal(event.Data, &ev); err != nil {
					continue
				}
				if ev.Delta.Type == "text_delta" {
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
				}

			case "message_stop":
				text := textBuf.String()
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
						Parts: []*genai.Part{{Text: fmt.Sprintf("[Server Error] %s", string(event.Data))}},
					},
					TurnComplete: true,
					FinishReason: genai.FinishReasonStop,
				}, nil)
				return
			}
		}

		if textBuf.Len() > 0 {
			yield(&model.LLMResponse{
				Content: &genai.Content{
					Role:  "model",
					Parts: []*genai.Part{{Text: textBuf.String()}},
				},
				TurnComplete: true,
				FinishReason: genai.FinishReasonStop,
			}, nil)
		}
	}
}

// extractPrompt gets the user's NEW text from the LAST content entry only.
// Returns "" for empty input, synthetic runner messages, or function responses.
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
			return "" // tool result from runner, skip
		}
		if part.Text != "" {
			// Skip runner's synthetic continuation message
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
