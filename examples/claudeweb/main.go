package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model/claudeweb"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
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
	llm.Shadow = &claudeweb.ShadowExecutor{
		WorkDir: workDir,
		Enabled: true,
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "claude_shadow_agent",
		Model:       llm,
		Description: "Claude via web API with local shadow execution",
		Instruction: "You are a helpful assistant.",
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	sessionSvc := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        "claudeweb",
		Agent:          a,
		SessionService: sessionSvc,
	})
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	// Create a session
	sess, err := sessionSvc.Create(ctx, &session.CreateRequest{
		AppName: "claudeweb",
		UserID:  "user",
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println("  Claude Shadow Agent")
	fmt.Printf("  Model: %s | Effort: %s\n", modelName, effort)
	fmt.Printf("  Shadow workdir: %s\n", workDir)
	fmt.Println("  Commands: /status  /new  /quit")
	fmt.Println("  Empty Enter = ignored (no message sent)")
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for {
		fmt.Print("You > ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())

		// Empty input: just re-prompt, don't send anything
		if input == "" {
			continue
		}

		// Commands
		switch input {
		case "/quit", "/exit", "/q":
			fmt.Println("Bye!")
			return
		case "/status":
			fmt.Printf("  Session: %s\n", sess.ID())
			fmt.Printf("  Shadow: %s (%v)\n", workDir, llm.Shadow.Enabled)
			fmt.Println()
			continue
		case "/new":
			// Start fresh conversation
			sess, err = sessionSvc.Create(ctx, &session.CreateRequest{
				AppName: "claudeweb",
				UserID:  "user",
			})
			if err != nil {
				fmt.Printf("  Error creating session: %v\n", err)
				continue
			}
			llm.ResetConversation()
			fmt.Println("  ✓ New conversation started")
			fmt.Println()
			continue
		}

		// Send to Claude
		fmt.Println()
		msg := genai.NewContentFromText(input, "user")

		for event, err := range r.Run(ctx, "user", sess.ID(), msg, agent.RunConfig{}) {
			if err != nil {
				fmt.Printf("\n[Error] %v\n", err)
				break
			}
			if event.Content() != nil {
				for _, part := range event.Content().Parts {
					if part.Text != "" {
						fmt.Print(part.Text)
					}
				}
			}
		}
		fmt.Println()
		fmt.Println()
	}
}
