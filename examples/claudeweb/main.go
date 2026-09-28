package main

import (
	"context"
	"log"
	"os"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/claudeweb"
)

func main() {
	ctx := context.Background()

	baseURL := os.Getenv("CLAUDE_WEB_BASE_URL")
	if baseURL == "" {
		baseURL = "https://c.aimonkey.plus"
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
	effort := os.Getenv("CLAUDE_WEB_EFFORT")
	if effort == "" {
		effort = "medium"
	}
	workDir := os.Getenv("CLAUDE_WEB_WORKDIR")
	if workDir == "" {
		workDir, _ = os.Getwd()
	}

	client := claudeweb.NewClient(claudeweb.ClientConfig{
		BaseURL: baseURL,
		OrgID:   orgID,
		Cookie:  cookie,
	})

	llm := claudeweb.NewModel(client, modelName, effort)

	// Enable shadow execution: mirror remote tool calls locally
	llm.Shadow = &claudeweb.ShadowExecutor{
		WorkDir: workDir,
		Enabled: true,
	}

	log.Printf("Shadow execution enabled, workdir=%s", workDir)

	a, err := llmagent.New(llmagent.Config{
		Name:        "claude_shadow_agent",
		Model:       llm,
		Description: "Claude via web API with local shadow execution",
		Instruction: "You are a helpful assistant. Execute code and commands as needed.",
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
