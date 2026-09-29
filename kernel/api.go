package kernel

import (
	"context"
	"fmt"
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
	Ctx    context.Context
	Cancel context.CancelFunc
}

var (
	taskMeta   = make(map[int64]*TaskMeta)
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
	}
	metaMu.Unlock()

	return pid, nil
}

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

	for i := 1; i < NR_TASKS; i++ {
		t := Task[i]
		if t == nil { continue }
		if pid >= 0 && int64(t.Pid) != pid { continue }
		schedMu.Unlock()
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
			Schedule()
		}
	}

	return 0, 0, fmt.Errorf("wait: no children")
}

func Kill(pid int64, sig int64) error {
	schedMu.Lock()
	defer schedMu.Unlock()

	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil && int64(Task[i].Pid) == pid {
			Task[i].Signal |= 1 << uint(sig-1)
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
	pid := int64(t.Pid)
	m := getMeta(pid)
	m.Pwd = pwd
}

func GetPwd(t *TaskStruct) string {
	pid := int64(t.Pid)
	m := getMeta(pid)
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

func SetStartupTime(t int64) {
	StartupTime = int32(t)
}
