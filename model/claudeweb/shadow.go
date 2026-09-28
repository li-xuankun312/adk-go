package claudeweb

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// ShadowExecutor intercepts tool_use blocks from the remote Claude
// and mirrors the commands locally.
type ShadowExecutor struct {
	WorkDir string // working directory for local execution
	Enabled bool
}

// toolInput represents the parsed input from different tool types
type toolInput struct {
	// bash_tool / computer style
	Command string `json:"command"`
	// repl style
	Code string `json:"code"`
	// artifacts / str_replace_editor style
	Path     string `json:"path"`
	Content  string `json:"content"`
	FileText string `json:"file_text"`
	OldStr   string `json:"old_str"`
	NewStr   string `json:"new_str"`
}

// Execute runs the tool locally based on the tool name and accumulated JSON input.
// Returns a description of what was done locally, or "" if nothing was executed.
func (s *ShadowExecutor) Execute(toolName string, inputJSON string) string {
	if !s.Enabled || inputJSON == "" {
		return ""
	}

	var input toolInput
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		log.Printf("[shadow] failed to parse input for %s: %v", toolName, err)
		return ""
	}

	switch {
	case isBashTool(toolName):
		cmd := input.Command
		if cmd == "" {
			cmd = input.Code
		}
		if cmd == "" {
			return ""
		}
		return s.execBash(cmd)

	case isCreateFile(toolName):
		path := input.Path
		content := input.Content
		if content == "" {
			content = input.FileText
		}
		if path == "" || content == "" {
			return ""
		}
		return s.execBash(fmt.Sprintf("cat > %s << 'SHADOWEOF'\n%s\nSHADOWEOF", path, content))

	case isStrReplace(toolName):
		if input.Path == "" || input.OldStr == "" {
			return ""
		}
		// Use sed-like approach
		log.Printf("[shadow] str_replace on %s (skipped - complex operation)", input.Path)
		return fmt.Sprintf("[shadow] str_replace on %s noted but skipped locally", input.Path)

	default:
		log.Printf("[shadow] unknown tool %q, skipping", toolName)
		return ""
	}
}

func (s *ShadowExecutor) execBash(command string) string {
	log.Printf("[shadow] ▶ executing: %s", truncateCmd(command, 120))

	cmd := exec.Command("bash", "-c", command)
	if s.WorkDir != "" {
		cmd.Dir = s.WorkDir
	}

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	result := stdout.String()
	errStr := stderr.String()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
			errStr = err.Error()
		}
	}

	// Truncate
	if len(result) > 2000 {
		result = result[:2000] + "\n... (truncated)"
	}

	status := "✓"
	if exitCode != 0 {
		status = fmt.Sprintf("✗ exit=%d", exitCode)
	}
	log.Printf("[shadow] %s stdout=%d bytes stderr=%d bytes",
		status, len(stdout.String()), len(stderr.String()))

	if errStr != "" && exitCode != 0 {
		return fmt.Sprintf("[shadow %s] %s\nstderr: %s", status, result, truncateCmd(errStr, 500))
	}
	return fmt.Sprintf("[shadow %s] %s", status, result)
}

func isBashTool(name string) bool {
	n := strings.ToLower(name)
	return n == "bash_tool" || n == "bash" || n == "repl" ||
		n == "computer" || n == "terminal" || n == "shell" ||
		n == "execute_command" || n == "run_command"
}

func isCreateFile(name string) bool {
	n := strings.ToLower(name)
	return n == "create_file" || n == "write_file" || n == "write_to_file"
}

func isStrReplace(name string) bool {
	n := strings.ToLower(name)
	return n == "str_replace" || n == "str_replace_editor" || n == "edit_file"
}

func truncateCmd(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
