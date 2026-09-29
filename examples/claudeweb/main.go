package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"google.golang.org/genai"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/kernel"
	"google.golang.org/adk/v2/model/claudeweb"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
)

type appConfig struct {
	baseURL   string
	orgID     string
	cookie    string
	modelName string
	effort    string
	workDir   string
}

var cfg *appConfig

func main() {
	cfg = &appConfig{
		baseURL:   envOr("CLAUDE_WEB_BASE_URL", "https://c.aimonkey.plus"),
		orgID:     envRequired("CLAUDE_WEB_ORG_ID"),
		cookie:    envRequired("CLAUDE_WEB_COOKIE"),
		modelName: envOr("CLAUDE_WEB_MODEL", "claude-opus-4-6"),
		effort:    envOr("CLAUDE_WEB_EFFORT", "medium"),
		workDir:   envOr("CLAUDE_WEB_WORKDIR", mustGetwd()),
	}

	kernel.Init()
	initTask := kernel.GetCurrent()
	kernel.SetPwd(initTask, cfg.workDir)

	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println("  Claude Shadow Agent — Linux 0.11 Process Model")
	fmt.Printf("  Model: %s | Effort: %s\n", cfg.modelName, cfg.effort)
	fmt.Printf("  WorkDir: %s\n", cfg.workDir)
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("  /spawn <prompt>   fork() + exec(prompt)")
	fmt.Println("  /wait <pid>       waitpid()")
	fmt.Println("  /kill <pid>       kill(pid, SIGKILL)")
	fmt.Println("  /ps               show_stat()")
	fmt.Println("  /new              reset main conversation")
	fmt.Println("  /quit             exit")
	fmt.Println("═══════════════════════════════════════════════")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	var mainLLM *claudeweb.Model
	var mainRunner *runner.Runner
	var mainSessID string
	mainLLM, mainRunner, mainSessID = newConversation("init")

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

		case input == "/ps":
			kernel.Ps()
			fmt.Println()

		case input == "/new":
			mainLLM, mainRunner, mainSessID = newConversation("init")
			fmt.Println("  ✓ Main conversation reset")
			fmt.Println()

		case strings.HasPrefix(input, "/spawn "):
			prompt := strings.TrimSpace(strings.TrimPrefix(input, "/spawn "))
			if prompt == "" {
				fmt.Println("  Usage: /spawn <prompt>")
				continue
			}
			pid, err := kernel.Fork(prompt, cfg.workDir)
			if err != nil {
				fmt.Printf("  fork() failed: %v\n", err)
				continue
			}
			fmt.Printf("\n┌─ [PID=%d] fork() ok, running...\n", pid)
			go runChildProcess(pid, prompt)
			fmt.Printf("│  (backgrounded — /wait %d to collect result)\n", pid)
			fmt.Printf("└─\n\n")

		case strings.HasPrefix(input, "/wait"):
			args := strings.TrimSpace(strings.TrimPrefix(input, "/wait"))
			var waitPid int64
			if args == "" {
				waitPid = -1
			} else {
				n, err := strconv.ParseInt(args, 10, 64)
				if err != nil {
					fmt.Printf("  Usage: /wait [pid]\n")
					continue
				}
				waitPid = n
			}
			fmt.Printf("  waitpid(%d)...\n", waitPid)
			childPid, code, err := kernel.Wait(waitPid)
			if err != nil {
				fmt.Printf("  waitpid error: %v\n", err)
			} else {
				fmt.Printf("  PID=%d exited with code %d\n", childPid, code)
				result := kernel.GetResult(childPid)
				if result != "" {
					fmt.Printf("  Result:\n%s\n", result)
				}
			}
			fmt.Println()

		case strings.HasPrefix(input, "/kill "):
			args := strings.TrimSpace(strings.TrimPrefix(input, "/kill "))
			n, err := strconv.ParseInt(args, 10, 64)
			if err != nil {
				fmt.Println("  Usage: /kill <pid>")
				continue
			}
			if err := kernel.Kill(n, kernel.SIG_KILL); err != nil {
				fmt.Printf("  kill() error: %v\n", err)
			} else {
				fmt.Printf("  kill(%d, SIGKILL) sent\n", n)
			}
			fmt.Println()

		default:
			fmt.Println()
			runPrompt(context.Background(), mainLLM, mainRunner, mainSessID, input, "")
			fmt.Println()
		}
	}
}

func runChildProcess(pid int64, prompt string) {
	llm, r, sessID := newConversation(fmt.Sprintf("pid_%d", pid))
	kernel.SetConvID(pid, llm.ConvID())

	var result strings.Builder
	ctx := context.Background()
	t := kernel.GetTask(pid)
	if t != nil {
		ctx = kernel.TaskCtx(t)
	}

	msg := genai.NewContentFromText(prompt, "user")
	for event, err := range r.Run(ctx, "user", sessID, msg, agent.RunConfig{}) {
		if err != nil {
			result.WriteString(fmt.Sprintf("[Error] %v", err))
			break
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
				if part.Text != "" {
					result.WriteString(part.Text)
				}
			}
		}
	}

	kernel.SetResult(pid, result.String())
	kernel.Exit(pid, 0)
	log.Printf("[PID=%d] process exited", pid)
}

func runPrompt(ctx context.Context, llm *claudeweb.Model, r *runner.Runner, sessID string, prompt string, prefix string) {
	msg := genai.NewContentFromText(prompt, "user")
	for event, err := range r.Run(ctx, "user", sessID, msg, agent.RunConfig{}) {
		if err != nil {
			fmt.Printf("%s[Error] %v\n", prefix, err)
			break
		}
		if event.Content != nil {
			for _, part := range event.Content.Parts {
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

func newConversation(name string) (*claudeweb.Model, *runner.Runner, string) {
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
		Name:        fmt.Sprintf("claude_%s", name),
		Model:       llm,
		Description: "Claude with shadow execution",
		Instruction: "You are a helpful assistant.",
	})
	if err != nil {
		log.Fatalf("agent create failed: %v", err)
	}
	sessSvc := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:        name,
		Agent:          a,
		SessionService: sessSvc,
	})
	if err != nil {
		log.Fatalf("runner create failed: %v", err)
	}
	sess, err := sessSvc.Create(context.Background(), &session.CreateRequest{
		AppName: name,
		UserID:  "user",
	})
	if err != nil {
		log.Fatalf("session create failed: %v", err)
	}
	return llm, r, sess.Session.ID()
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
