package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/cmd/launcher"
	"google.golang.org/adk/v2/cmd/launcher/full"
	"google.golang.org/adk/v2/model/claudeweb"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// --- bash_tool ---
type bashInput struct {
	Command string `json:"command" description:"The bash command to execute"`
}

type bashOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func bashTool(_ agent.Context, in bashInput) (bashOutput, error) {
	log.Printf("[bash_tool] executing: %s", in.Command)
	cmd := exec.Command("bash", "-c", in.Command)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return bashOutput{Stderr: err.Error(), ExitCode: 1}, nil
		}
	}
	result := bashOutput{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
	}
	// Truncate long output
	if len(result.Stdout) > 10000 {
		result.Stdout = result.Stdout[:10000] + "\n... (truncated)"
	}
	if len(result.Stderr) > 5000 {
		result.Stderr = result.Stderr[:5000] + "\n... (truncated)"
	}
	return result, nil
}

// --- read_file ---
type readFileInput struct {
	Path string `json:"path" description:"File path to read"`
}

type readFileOutput struct {
	Content string `json:"content"`
	Error   string `json:"error,omitempty"`
}

func readFileTool(_ agent.Context, in readFileInput) (readFileOutput, error) {
	log.Printf("[read_file] reading: %s", in.Path)
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return readFileOutput{Error: err.Error()}, nil
	}
	content := string(data)
	if len(content) > 20000 {
		content = content[:20000] + "\n... (truncated)"
	}
	return readFileOutput{Content: content}, nil
}

// --- write_file ---
type writeFileInput struct {
	Path    string `json:"path" description:"File path to write"`
	Content string `json:"content" description:"Content to write"`
}

type writeFileOutput struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func writeFileTool(_ agent.Context, in writeFileInput) (writeFileOutput, error) {
	log.Printf("[write_file] writing: %s (%d bytes)", in.Path, len(in.Content))
	err := os.WriteFile(in.Path, []byte(in.Content), 0644)
	if err != nil {
		return writeFileOutput{Error: err.Error()}, nil
	}
	return writeFileOutput{Success: true}, nil
}

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

	client := claudeweb.NewClient(claudeweb.ClientConfig{
		BaseURL: baseURL,
		OrgID:   orgID,
		Cookie:  cookie,
	})

	llm := claudeweb.NewModel(client, modelName, effort)

	bashT, err := functiontool.New(functiontool.Config{
		Name:        "bash_tool",
		Description: "Execute a bash command on the local server and return stdout, stderr, and exit code.",
	}, bashTool)
	if err != nil {
		log.Fatalf("Failed to create bash_tool: %v", err)
	}

	readT, err := functiontool.New(functiontool.Config{
		Name:        "read_file",
		Description: "Read the contents of a file at the given path.",
	}, readFileTool)
	if err != nil {
		log.Fatalf("Failed to create read_file: %v", err)
	}

	writeT, err := functiontool.New(functiontool.Config{
		Name:        "write_file",
		Description: "Write content to a file at the given path.",
	}, writeFileTool)
	if err != nil {
		log.Fatalf("Failed to create write_file: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:  "claude_code_agent",
		Model: llm,
		Description: "An agent that can execute bash commands and read/write files on the local server.",
		Instruction: fmt.Sprintf(`You are a coding agent running on a Linux server. You have access to tools that let you execute bash commands, read files, and write files on this server.

Current working directory: %s

When the user asks you to do something, use the tools to actually do it. Do not just describe what you would do - actually execute the commands.

For example, if asked to clone a repo, use bash_tool to run git clone.
If asked to read code, use read_file.
If asked to create or modify files, use write_file.`, getCurrentDir()),
		Tools: []tool.Tool{bashT, readT, writeT},
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

func getCurrentDir() string {
	dir, _ := os.Getwd()
	return dir
}
