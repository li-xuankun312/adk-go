package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model/claudeweb"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
)

var pidCounter atomic.Int32

type appConfig struct {
	baseURL   string
	orgID     string
	cookie    string
	modelName string
	effort    string
	workDir   string
}

func main() {
	ctx := context.Background()

	cfg := &appConfig{
		baseURL:   envOr("CLAUDE_WEB_BASE_URL", "https://c.aimonkey.plus"),
		orgID:     envRequired("CLAUDE_WEB_ORG_ID"),
		cookie:    envRequired("CLAUDE_WEB_COOKIE"),
		modelName: envOr("CLAUDE_WEB_MODEL", "claude-opus-4-6"),
		effort:    envOr("CLAUDE_WEB_EFFORT", "medium"),
		workDir:   envOr("CLAUDE_WEB_WORKDIR", mustGetwd()),
	}

	mainProc := newProcess(cfg, 0)

	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println("  Claude Shadow Agent (Process Model)")
	fmt.Printf("  Model: %s | Effort: %s\n", cfg.modelName, cfg.effort)
	fmt.Printf("  Shadow workdir: %s\n", cfg.workDir)
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("  /spawn <prompt>  Start a child process")
	fmt.Println("  /new             Reset main conversation")
	fmt.Println("  /status          Show state")
	fmt.Println("  /quit            Exit")
	fmt.Println("  Empty Enter      Ignored")
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

		if input == "" {
			continue
		}

		switch {
		case input == "/quit" || input == "/exit" || input == "/q":
			fmt.Println("Bye!")
			return

		case input == "/status":
			fmt.Printf("  Main PID=0 convID=%s\n", mainProc.getConvID())
			fmt.Printf("  Shadow workdir: %s\n", cfg.workDir)
			fmt.Printf("  Total spawned: %d\n", pidCounter.Load())
			fmt.Println()

		case input == "/new":
			mainProc = newProcess(cfg, 0)
			fmt.Println("  ✓ Main conversation reset")
			fmt.Println()

		case strings.HasPrefix(input, "/spawn "):
			prompt := strings.TrimPrefix(input, "/spawn ")
			prompt = strings.TrimSpace(prompt)
			if prompt == "" {
				fmt.Println("  Usage: /spawn <prompt>")
				continue
			}
			pid := int(pidCounter.Add(1))
			fmt.Printf("\n┌─ [PID=%d] Spawning child process...\n", pid)
			child := newProcess(cfg, pid)
			child.run(ctx, prompt, "│  ")
			fmt.Printf("└─ [PID=%d] Child process ended\n\n", pid)

		default:
			fmt.Println()
			mainProc.run(ctx, input, "")
			fmt.Println()
		}
	}
}

// process represents an independent Claude conversation with shadow execution
type process struct {
	pid     int
	llm     *claudeweb.Model
	r       *runner.Runner
	sessID  string
	appName string
}

func newProcess(cfg *appConfig, pid int) *process {
	client := claudeweb.NewClient(claudeweb.ClientConfig{
		BaseURL: cfg.baseURL,
		OrgID:   cfg.orgID,
		Cookie:  cfg.cookie,
	})

	llm := claudeweb.NewModel(client, cfg.modelName, cfg.effort)
	llm.Shadow = &claudeweb.ShadowExecutor{
		WorkDir: cfg.workDir,
		Enabled: true,
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        fmt.Sprintf("claude_pid_%d", pid),
		Model:       llm,
		Description: "Claude with shadow execution",
		Instruction: "You are a helpful assistant.",
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	sessSvc := session.InMemoryService()
	appName := fmt.Sprintf("proc_%d", pid)
	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          a,
		SessionService: sessSvc,
	})
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	sess, err := sessSvc.Create(context.Background(), &session.CreateRequest{
		AppName: appName,
		UserID:  "user",
	})
	if err != nil {
		log.Fatalf("Failed to create session: %v", err)
	}

	return &process{
		pid:     pid,
		llm:     llm,
		r:       r,
		sessID:  sess.ID(),
		appName: appName,
	}
}

func (p *process) getConvID() string {
	return p.llm.ConvID()
}

func (p *process) run(ctx context.Context, prompt string, prefix string) {
	msg := genai.NewContentFromText(prompt, "user")
	for event, err := range p.r.Run(ctx, "user", p.sessID, msg, agent.RunConfig{}) {
		if err != nil {
			fmt.Printf("%s[Error] %v\n", prefix, err)
			break
		}
		if event.Content() != nil {
			for _, part := range event.Content().Parts {
				if part.Text != "" {
					if prefix != "" {
						for _, line := range strings.Split(part.Text, "\n") {
							fmt.Printf("%s%s\n", prefix, line)
						}
					} else {
						fmt.Print(part.Text)
					}
				}
			}
		}
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envRequired(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("set %s", key)
	}
	return v
}

func mustGetwd() string {
	d, _ := os.Getwd()
	return d
}
