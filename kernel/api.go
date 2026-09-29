package kernel

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	. "google.golang.org/adk/v2/include"
)

const (
	SIG_KILL int64 = 9
)

type TaskMeta struct {
	Prompt string
	Pwd    string
	ConvID string
	Result string
	StdErr string
	Ctx    context.Context
	Cancel context.CancelFunc
	Cmd    *exec.Cmd
	stdout strings.Builder
	stderr strings.Builder
	Done   chan struct{}
}

var (
	taskMeta = make(map[int64]*TaskMeta)
	metaMu   sync.Mutex
	nextPid  int64 = 1
	pidMu    sync.Mutex
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

func Init() {
	SchedInit()
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

func Fork(prompt string, pwd string) (int64, error) {
	pid := allocPid()

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

	child := &TaskStruct{}
	*child = *Current
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

	ctx, cancel := context.WithCancel(context.Background())
	metaMu.Lock()
	taskMeta[pid] = &TaskMeta{
		Prompt: prompt,
		Pwd:    pwd,
		Ctx:    ctx,
		Cancel: cancel,
		Done:   make(chan struct{}),
	}
	metaMu.Unlock()

	return pid, nil
}

func ForkExec(command string, workDir string) (int64, error) {
	pid := allocPid()

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
		return 0, fmt.Errorf("fork: %d tasks full", NR_TASKS)
	}

	child := &TaskStruct{}
	child.Pid = int32(pid)
	child.Father = Current.Pid
	child.State = TASK_RUNNING
	child.Counter = 15
	child.Priority = 15
	child.Signal = 0

	Task[slot] = child
	schedMu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	if workDir != "" {
		cmd.Dir = workDir
	}

	meta := &TaskMeta{
		Prompt: command,
		Pwd:    workDir,
		Ctx:    ctx,
		Cancel: cancel,
		Cmd:    cmd,
		Done:   make(chan struct{}),
	}
	cmd.Stdout = &meta.stdout
	cmd.Stderr = &meta.stderr

	metaMu.Lock()
	taskMeta[pid] = meta
	metaMu.Unlock()

	err := cmd.Start()
	if err != nil {
		schedMu.Lock()
		Task[slot] = nil
		schedMu.Unlock()
		cancel()
		return 0, fmt.Errorf("exec: %v", err)
	}

	go func() {
		werr := cmd.Wait()
		exitCode := 0
		if werr != nil {
			if exitErr, ok := werr.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}

		metaMu.Lock()
		meta.Result = meta.stdout.String()
		meta.StdErr = meta.stderr.String()
		metaMu.Unlock()

		schedMu.Lock()
		for i := 1; i < NR_TASKS; i++ {
			if Task[i] != nil && int64(Task[i].Pid) == pid {
				Task[i].State = TASK_ZOMBIE
				Task[i].ExitCode = int32(exitCode)
				break
			}
		}
		schedMu.Unlock()

		close(meta.Done)
	}()

	return pid, nil
}

func Exit(pid int64, code int) {
	schedMu.Lock()
	defer schedMu.Unlock()
	for i := 1; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == pid {
			Task[i].State = TASK_ZOMBIE
			Task[i].ExitCode = int32(code)
			metaMu.Lock()
			if m, ok := taskMeta[pid]; ok && m.Done != nil {
				select {
				case <-m.Done:
				default:
					close(m.Done)
				}
			}
			metaMu.Unlock()
			return
		}
	}
}

func Wait(pid int64) (int64, int, error) {
	schedMu.Lock()
	var targetSlot int = -1
	var targetPid int64 = -1
	for i := 1; i < NR_TASKS; i++ {
		t := Task[i]
		if t == nil {
			continue
		}
		if pid >= 0 && int64(t.Pid) != pid {
			continue
		}
		targetSlot = i
		targetPid = int64(t.Pid)
		if t.State == TASK_ZOMBIE {
			code := int(t.ExitCode)
			Task[i] = nil
			schedMu.Unlock()
			return targetPid, code, nil
		}
		break
	}
	schedMu.Unlock()

	if targetPid < 0 {
		return 0, 0, fmt.Errorf("wait: no children")
	}

	metaMu.Lock()
	meta := taskMeta[targetPid]
	metaMu.Unlock()

	if meta != nil && meta.Done != nil {
		<-meta.Done
	}

	schedMu.Lock()
	defer schedMu.Unlock()
	if targetSlot >= 0 && targetSlot < NR_TASKS && Task[targetSlot] != nil && int64(Task[targetSlot].Pid) == targetPid {
		code := int(Task[targetSlot].ExitCode)
		Task[targetSlot] = nil
		return targetPid, code, nil
	}
	for i := 1; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == targetPid {
			code := int(Task[i].ExitCode)
			Task[i] = nil
			return targetPid, code, nil
		}
	}
	return 0, 0, fmt.Errorf("wait: child %d disappeared", targetPid)
}

func Kill(pid int64, sig int64) error {
	schedMu.Lock()
	found := false
	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == pid {
			Task[i].Signal |= int32(1 << uint(sig-1))
			found = true
			break
		}
	}
	schedMu.Unlock()

	if !found {
		return fmt.Errorf("kill: no such process %d", pid)
	}

	if sig == int64(SIGKILL) {
		metaMu.Lock()
		m := taskMeta[pid]
		metaMu.Unlock()
		if m != nil {
			if m.Cmd != nil && m.Cmd.Process != nil {
				m.Cmd.Process.Kill()
			}
			if m.Cancel != nil {
				m.Cancel()
			}
		}
	}

	return nil
}

func Ps() {
	schedMu.Lock()
	defer schedMu.Unlock()

	fmt.Printf("  %-6s %-6s %-10s %-8s %s\n", "PID", "PPID", "STATE", "CONVID", "CMD")
	fmt.Printf("  %-6s %-6s %-10s %-8s %s\n", "---", "----", "-----", "------", "---")
	for i := 0; i < NR_TASKS; i++ {
		t := Task[i]
		if t == nil {
			continue
		}
		stateName := "?"
		switch t.State {
		case TASK_RUNNING:
			stateName = "R"
		case TASK_INTERRUPTIBLE:
			stateName = "S"
		case TASK_UNINTERRUPTIBLE:
			stateName = "D"
		case TASK_ZOMBIE:
			stateName = "Z"
		case TASK_STOPPED:
			stateName = "T"
		}
		pid := int64(t.Pid)
		metaMu.Lock()
		m := taskMeta[pid]
		prompt := ""
		convID := ""
		if m != nil {
			prompt = m.Prompt
			convID = m.ConvID
			if len(prompt) > 60 {
				prompt = prompt[:60] + "..."
			}
			if len(convID) > 8 {
				convID = convID[:8]
			}
		}
		metaMu.Unlock()
		fmt.Printf("  %-6d %-6d %-10s %-8s %s\n", t.Pid, t.Father, stateName, convID, prompt)
	}
}

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

func GetCurrent() *TaskStruct {
	schedMu.Lock()
	defer schedMu.Unlock()
	return Current
}

func SetPwd(t *TaskStruct, pwd string) {
	m := getMeta(int64(t.Pid))
	m.Pwd = pwd
}

func GetPwd(t *TaskStruct) string {
	m := getMeta(int64(t.Pid))
	return m.Pwd
}

func TaskCtx(t *TaskStruct) context.Context {
	pid := int64(t.Pid)
	metaMu.Lock()
	defer metaMu.Unlock()
	if m, ok := taskMeta[pid]; ok && m.Ctx != nil {
		return m.Ctx
	}
	return context.Background()
}

func SetConvID(pid int64, convID string) {
	m := getMeta(pid)
	m.ConvID = convID
}

func SetResult(pid int64, result string) {
	m := getMeta(pid)
	m.Result = result
}

func GetResult(pid int64) string {
	metaMu.Lock()
	defer metaMu.Unlock()
	if m, ok := taskMeta[pid]; ok {
		return m.Result
	}
	return ""
}

func GetStderr(pid int64) string {
	metaMu.Lock()
	defer metaMu.Unlock()
	if m, ok := taskMeta[pid]; ok {
		return m.StdErr
	}
	return ""
}

func SetStartupTime(t int64) {
	StartupTime = int32(t)
}
