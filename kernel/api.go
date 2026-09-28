// kernel/api.go
//
// Public API for the kernel package.
// Wraps internal functions with exported names and proper locking.

package kernel

import "fmt"

// Init initializes the kernel. Must be called before any other function.
// Creates the init process (task[0]).
func Init() {
	sched_init()
}

// Fork creates a new child process with the given prompt.
// Returns the child's PID.
// Mirrors the fork() system call → find_empty_process + copy_process.
func Fork(prompt string, pwd string) (int64, error) {
	pid, err := copy_process(prompt)
	if err != nil {
		return 0, err
	}
	// Set the working directory for the child
	mu.Lock()
	for i := 1; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			if pwd != "" {
				task[i].pwd = pwd
			}
			break
		}
	}
	mu.Unlock()
	return pid, nil
}

// Exit terminates the process with the given PID.
// Mirrors the exit() system call.
func Exit(pid int64, code int) {
	mu.Lock()
	defer mu.Unlock()

	saved := current
	for i := 1; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			current = task[i]
			do_exit(code)
			current = saved
			return
		}
	}
	current = saved
}

// Wait waits for a child process to exit.
// Mirrors the waitpid() system call.
// Returns (child_pid, exit_code, error).
func Wait(pid int64) (int64, int, error) {
	return sys_waitpid(pid, 0)
}

// WaitNoHang checks if a child has exited without blocking.
func WaitNoHang(pid int64) (int64, int, error) {
	return sys_waitpid(pid, WNOHANG)
}

// Kill sends a signal to a process.
// Mirrors the kill() system call.
func Kill(pid int64, sig int64) error {
	ret := sys_kill(pid, sig)
	if ret != 0 {
		return fmt.Errorf("kill: error %d", ret)
	}
	return nil
}

// Ps displays all active processes (like the ps command).
func Ps() {
	show_stat()
}

// GetTask returns the task_struct for a given PID, or nil.
func GetTask(pid int64) *task_struct {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			return task[i]
		}
	}
	return nil
}

// Current returns the current process.
func Current() *task_struct {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// SetCurrent sets the current process (used when switching context).
func SetCurrent(p *task_struct) {
	mu.Lock()
	defer mu.Unlock()
	current = p
}

// GetPwd returns the working directory of a process.
func GetPwd(pid int64) string {
	t := GetTask(pid)
	if t == nil {
		return ""
	}
	return t.pwd
}

// SetConvID sets the conversation ID for a process.
func SetConvID(pid int64, convID string) {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			task[i].conv_id = convID
			return
		}
	}
}

// SetResult stores the output of a completed process.
func SetResult(pid int64, result string) {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			task[i].result = result
			return
		}
	}
}

// GetResult returns the stored output of a process.
func GetResult(pid int64) string {
	t := GetTask(pid)
	if t == nil {
		return ""
	}
	return t.result
}

// Pipe creates a pipe for inter-process communication.
func Pipe() (*pipe_struct, error) {
	return sys_pipe()
}
