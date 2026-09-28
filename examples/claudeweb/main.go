package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/claudeweb"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

type weatherInput struct {
	City string `json:"city" description:"The city name"`
}

type weatherOutput struct {
	Report string `json:"report"`
}

func getWeather(_ agent.Context, in weatherInput) (weatherOutput, error) {
	return weatherOutput{
		Report: fmt.Sprintf("It is currently 25°C and sunny in %s.", in.City),
	}, nil
}

func main() {
	ctx := context.Background()

	// Configuration from environment
	baseURL := os.Getenv("CLAUDE_WEB_BASE_URL")
	if baseURL == "" {
		baseURL = "https://chk.aimonkey.plus"
	}
	orgID := os.Getenv("CLAUDE_WEB_ORG_ID")
	if orgID == "" {
		log.Fatal("set CLAUDE_WEB_ORG_ID")
	}
	cookie := os.Getenv("CLAUDE_WEB_COOKIE")
	if cookie == "" {
		log.Fatal("set CLAUDE_WEB_COOKIE")
	}
	modelName := os.Getenv("CLAUDE_WEB_MODEL")
	if modelName == "" {
		modelName = "claude-opus-4-6"
	}

	client := claudeweb.NewClient(claudeweb.ClientConfig{
		BaseURL: baseURL,
		OrgID:   orgID,
		Cookie:  cookie,
	})

	llm := claudeweb.NewModel(client, modelName)

	weatherTool, err := functiontool.New(functiontool.Config{
		Name:        "get_weather",
		Description: "Returns the current weather for a given city.",
	}, getWeather)
	if err != nil {
		log.Fatalf("Failed to create tool: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "claude_web_agent",
		Model:       llm,
		Description: "An agent powered by Claude via web API proxy.",
		Instruction: "You are a helpful assistant. When asked about the weather in a city, call the get_weather tool and report the result.",
		Tools:       []tool.Tool{weatherTool},
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(a),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}
