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

	// Conversation state: maps adk session context to remote conversation UUID.
	// For simplicity, we use a single conversation per Model instance.
	// For multi-session support, this should be keyed by session ID.
	mu             sync.Mutex
	convID         string // current conversation UUID
	lastAssistantUUID string // parent_message_uuid for tool results
}

// NewModel creates a Model backed by the claude.ai web API.
func NewModel(client *Client, modelName string) *Model {
	return &Model{
		client:    client,
		modelName: modelName,
	}
}

func (m *Model) Name() string { return m.modelName }

// GenerateContent implements model.LLM.
// It converts the adk-go request into a web API call, parses the SSE stream,
// and yields LLMResponse events that the adk-go runner understands.
func (m *Model) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if req == nil {
		return singleError(fmt.Errorf("claudeweb: nil request"))
	}

	// Determine what to send: new prompt or tool results
	webReq, isToolResult, err := m.buildRequest(req)
	if err != nil {
		return singleError(fmt.Errorf("claudeweb: build request: %w", err))
	}

	m.mu.Lock()
	convID := m.convID
	if convID == "" {
		// First turn: create a new conversation
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

	return func(yield func(*model.LLMResponse, error) bool) {
		body, err := m.client.Completion(convID, webReq)
		if err != nil {
			yield(nil, err)
			return
		}
		defer body.Close()

		events := ParseSSEStream(body)

		// Track content blocks being built
		var blocks []contentBlockState
		var stopReason string

		for event := range events {
			// Check context cancellation
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			default:
			}

			switch event.Event {
			case "ping":
				continue

			case "conversation_ready":
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
				// Expand blocks slice if needed
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
					// Yield partial text for streaming
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
				// Block is complete, nothing to do here yet

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
				// Build the final response from all accumulated blocks
				resp := buildFinalResponse(blocks, stopReason)
				resp.TurnComplete = true
				if !yield(resp, nil) {
					return
				}
				return

			case "message_limit":
				// Rate limit info, log but don't error
				log.Printf("claudeweb: message_limit event received")

			case "error":
				yield(nil, fmt.Errorf("claudeweb: server error: %s", string(event.Data)))
				return

			default:
				log.Printf("claudeweb: unknown event type: %s", event.Event)
			}
		}

		// If we got here without message_stop, yield what we have
		if len(blocks) > 0 {
			resp := buildFinalResponse(blocks, stopReason)
			resp.TurnComplete = true
			yield(resp, nil)
		}
	}
}

// buildRequest converts an adk-go LLMRequest into a web API CompletionRequest.
// Returns the request, whether it's a tool result turn, and any error.
func (m *Model) buildRequest(req *model.LLMRequest) (*CompletionRequest, bool, error) {
	webReq := &CompletionRequest{
		Model:         m.modelName,
		Timezone:      "Asia/Shanghai",
		Locale:        "en-US",
		Effort:        "high",
		ThinkingMode:  "off",
		RenderingMode: "messages",
		Attachments:   []json.RawMessage{},
		Files:         []json.RawMessage{},
		SyncSources:   []json.RawMessage{},
	}

	// Convert tools from adk-go format to web API format
	webReq.Tools = convertTools(req.Config)

	// Find the last user message to use as prompt, or tool results
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

	// Normal user message: extract text
	var prompt strings.Builder
	for _, part := range lastContent.Parts {
		if part.Text != "" {
			prompt.WriteString(part.Text)
		}
	}
	webReq.Prompt = prompt.String()

	return webReq, false, nil
}

// convertTools converts genai tool config to web API tool format.
func convertTools(config *genai.GenerateContentConfig) []WebTool {
	if config == nil {
		return []WebTool{}
	}

	var webTools []WebTool
	for _, tool := range config.Tools {
		for _, fd := range tool.FunctionDeclarations {
			schema, _ := json.Marshal(fd.Parameters)
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

// contentBlockState tracks the state of a content block being streamed.
type contentBlockState struct {
	blockType string // "text" or "tool_use"
	id        string // tool_use ID
	name      string // tool name
	text      strings.Builder
	inputJSON strings.Builder
}

// buildFinalResponse assembles the final LLMResponse from completed content blocks.
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

	resp := &model.LLMResponse{
		Content: content,
	}

	// Map stop reasons
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
