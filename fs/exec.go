package fs
import (
	"os"
	"os/exec"
)
func do_execve(filename string, argv []string, envp []string) error {
	_, err := os.Stat(filename)
	if err != nil { return err }
	cmd := exec.Command(filename, argv[1:]...)
	cmd.Env = envp
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func DoExecve(filename string, argv, envp []string) error { return do_execve(filename, argv, envp) }
