// kernel/api.go — High-level API facade for claudeweb/main.go
//
// Bridges the Linux 0.11 kernel internals (sched.go, fork.go, exit.go, etc.)
// with the Go-friendly API that the Claude shadow agent needs:
//   Init(), Fork(prompt, pwd), Wait(pid), Kill(pid, sig), Ps(),
//   Current(), GetTask(pid), SetConvID, SetResult, GetResult, etc.
//
// This file adds Go-specific fields (result, conv_id, pwd, ctx) to the
// task management layer without modifying the core Linux 0.11 structs.
package kernel

import (
	"context"
	"fmt"
	"sync"

	. "google.golang.org/adk/v2/include"
)

// Re-export constants that claudeweb/main.go references as kernel.XXX.
// Dot-imported names from include are available inside kernel/ but
// are not re-exported to other packages, so we explicitly alias them.
const (
	SIG_KILL int64 = 9  // kernel.SIG_KILL for claudeweb Kill() calls
)

// Extended task metadata (Go-specific, not part of Linux 0.11)
type TaskMeta struct {
	Prompt string
	Pwd    string
	ConvID string
	Result string
	Ctx    context.Context
	Cancel context.CancelFunc
}

var (
	taskMeta   = make(map[int64]*TaskMeta) // pid → metadata
	metaMu     sync.Mutex
	nextPid    int64 = 1
	pidMu      sync.Mutex
)

func allocPid() int64 {
	pidMu.Lock()
	defer pidMu.Unlock()
	p := nextPid
	nextPid++
	return p
}

func getMeta(pid int64) *TaskMeta {
	metaMu.Lock()
	defer metaMu.Unlock()
	m, ok := taskMeta[pid]
	if !ok {
		m = &TaskMeta{}
		taskMeta[pid] = m
	}
	return m
}

// Init initializes the kernel scheduler and sets up task 0
func Init() {
	SchedInit()
	// Set up task 0 (the idle/init task)
	if Task[0] == nil {
		Task[0] = &TaskStruct{}
	}
	Current = Task[0]
	Current.State = TASK_RUNNING
	Current.Pid = 0
	Current.Father = 0
	Current.Tty = 0
	taskMeta[0] = &TaskMeta{Pwd: "."}
}

// Fork creates a new task with a prompt
func Fork(prompt string, pwd string) (int64, error) {
	pid := allocPid()
	
	// Find a free slot in Task[]
	schedMu.Lock()
	slot := -1
	for i := 1; i < NR_TASKS; i++ {
		if Task[i] == nil {
			slot = i
			break
		}
	}
	if slot < 0 {
		schedMu.Unlock()
		return 0, fmt.Errorf("fork: no free task slots")
	}
	
	// Create the new task
	child := &TaskStruct{}
	*child = *Current // copy parent
	child.Pid = int32(pid)
	child.Father = Current.Pid
	child.State = TASK_RUNNING
	child.Counter = int32(Current.Counter >> 1)
	Current.Counter >>= 1
	child.Signal = 0
	child.Alarm = 0
	child.Leader = 0
	child.UsedMath = 0
	
	Task[slot] = child
	schedMu.Unlock()
	
	// Set up Go-specific metadata
	ctx, cancel := context.WithCancel(context.Background())
	metaMu.Lock()
	taskMeta[pid] = &TaskMeta{
		Prompt: prompt,
		Pwd:    pwd,
		Ctx:    ctx,
		Cancel: cancel,
	}
	metaMu.Unlock()
	
	return pid, nil
}

// Exit marks a task as zombie
func Exit(pid int64, code int) {
	schedMu.Lock()
	defer schedMu.Unlock()
	
	for i := 1; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == pid {
			Task[i].State = TASK_ZOMBIE
			Task[i].ExitCode = int32(code)
			return
		}
	}
}

// Wait waits for a child process (or any child if pid == -1)
func Wait(pid int64) (int64, int, error) {
	schedMu.Lock()
	defer schedMu.Unlock()
	
	for i := 1; i < NR_TASKS; i++ {
		t := Task[i]
		if t == nil { continue }
		if pid >= 0 && int64(t.Pid) != pid { continue }
		if t.State == TASK_ZOMBIE {
			childPid := int64(t.Pid)
			code := int(t.ExitCode)
			Task[i] = nil
			return childPid, code, nil
		}
	}
	
	// Look for any non-zombie child we should wait on
	for i := 1; i < NR_TASKS; i++ {
		t := Task[i]
		if t == nil { continue }
		if pid >= 0 && int64(t.Pid) != pid { continue }
		// Child exists but hasn't exited yet
		schedMu.Unlock()
		// Busy-wait (simplified; real kernel would use sleep_on)
		for {
			schedMu.Lock()
			if Task[i] == nil || Task[i].State == TASK_ZOMBIE {
				if Task[i] != nil {
					childPid := int64(Task[i].Pid)
					code := int(Task[i].ExitCode)
					Task[i] = nil
					schedMu.Unlock()
					return childPid, code, nil
				}
				schedMu.Unlock()
				return 0, 0, fmt.Errorf("wait: child disappeared")
			}
			schedMu.Unlock()
			// yield
			Schedule()
		}
	}
	
	return 0, 0, fmt.Errorf("wait: no children")
}

// Kill sends a signal to a process
func Kill(pid int64, sig int64) error {
	schedMu.Lock()
	defer schedMu.Unlock()
	
	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == pid {
			Task[i].Signal |= 1 << uint(sig-1)
			// If SIGKILL, also cancel Go context
			if sig == int64(SIGKILL) {
				metaMu.Lock()
				if m, ok := taskMeta[pid]; ok && m.Cancel != nil {
					m.Cancel()
				}
				metaMu.Unlock()
			}
			return nil
		}
	}
	return fmt.Errorf("kill: no such process %d", pid)
}

// Ps prints the process table
func Ps() {
	schedMu.Lock()
	defer schedMu.Unlock()
	
	fmt.Printf("  %-6s %-6s %-10s %-8s %s\n", "PID", "PPID", "STATE", "CONVID", "PROMPT")
	fmt.Printf("  %-6s %-6s %-10s %-8s %s\n", "---", "----", "-----", "------", "------")
	for i := 0; i < NR_TASKS; i++ {
		t := Task[i]
		if t == nil { continue }
		stateName := "unknown"
		switch t.State {
		case TASK_RUNNING: stateName = "RUNNING"
		case TASK_INTERRUPTIBLE: stateName = "SLEEPING"
		case TASK_UNINTERRUPTIBLE: stateName = "BLOCKED"
		case TASK_ZOMBIE: stateName = "ZOMBIE"
		case TASK_STOPPED: stateName = "STOPPED"
		}
		pid := int64(t.Pid)
		metaMu.Lock()
		m := taskMeta[pid]
		prompt := ""
		convID := ""
		if m != nil {
			prompt = m.Prompt
			convID = m.ConvID
			if len(prompt) > 40 { prompt = prompt[:40] + "..." }
			if len(convID) > 8 { convID = convID[:8] }
		}
		metaMu.Unlock()
		fmt.Printf("  %-6d %-6d %-10s %-8s %s\n", t.Pid, t.Father, stateName, convID, prompt)
	}
}

// GetTask returns the task struct for a pid
func GetTask(pid int64) *TaskStruct {
	schedMu.Lock()
	defer schedMu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == pid {
			return Task[i]
		}
	}
	return nil
}

// Current returns the current task (API for claudeweb; shadows include.Current variable)
// Note: the include package has a `Current` variable, and this function
// provides the claudeweb-compatible `kernel.Current()` call.
func GetCurrent() *TaskStruct {
	schedMu.Lock()
	defer schedMu.Unlock()
	return Current
}

// SetPwd sets the working directory for a task
func (t *TaskStruct) SetPwd(pwd string) {
	pid := int64(t.Pid)
	m := getMeta(pid)
	m.Pwd = pwd
}

// GetPwd returns the working directory
func (t *TaskStruct) GetPwd() string {
	pid := int64(t.Pid)
	m := getMeta(pid)
	return m.Pwd
}

// Ctx returns the context for the task
func (t *TaskStruct) Ctx() context.Context {
	pid := int64(t.Pid)
	metaMu.Lock()
	defer metaMu.Unlock()
	if m, ok := taskMeta[pid]; ok && m.Ctx != nil {
		return m.Ctx
	}
	return context.Background()
}

// SetConvID stores the conversation ID for a task
func SetConvID(pid int64, convID string) {
	m := getMeta(pid)
	m.ConvID = convID
}

// SetResult stores the result of a task
func SetResult(pid int64, result string) {
	m := getMeta(pid)
	m.Result = result
}

// GetResult retrieves the result
func GetResult(pid int64) string {
	metaMu.Lock()
	defer metaMu.Unlock()
	if m, ok := taskMeta[pid]; ok {
		return m.Result
	}
	return ""
}

// SetStartupTime sets the kernel boot timestamp
func SetStartupTime(t int64) {
	StartupTime = int32(t)
}
