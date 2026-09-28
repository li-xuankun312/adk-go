package kernel
import (
	"context"
	"sync"
	"time"
)
const (
	NR_TASKS = 64 // Max number of concurrent processes
	NR_OPEN  = 32 // Max open files per process (unused for now)
)
const (
	TASK_RUNNING         = 0
	TASK_INTERRUPTIBLE   = 1
	TASK_UNINTERRUPTIBLE = 2
	TASK_ZOMBIE          = 3
	TASK_STOPPED         = 4
)
type sigaction struct {
	sa_handler func(int)
	sa_mask    int64
	sa_flags   int
}
type task_struct struct {
	/* these are hardcoded - don't touch */
	state    int64 // -1 unrunnable, 0 runnable, >0 stopped
	counter  int64 // time slice remaining
	priority int64 // scheduling priority (initial counter value)
	signal   int64 // bitmap of pending signals
	sigaction [32]sigaction
	blocked  int64 // bitmap of masked signals
	/* various fields */
	exit_code int
	pid     int64
	father  int64
	pgrp    int64
	session int64
	leader  int64
	alarm      int64 // jiffies until alarm (unused for now)
	utime      int64 // user-mode time (ticks spent waiting for Claude)
	stime      int64 // system-mode time (ticks spent in shadow exec)
	cutime     int64 // children's utime
	cstime     int64 // children's stime
	start_time int64 // jiffies at process creation
	/* file system info — simplified */
	tty   int    // controlling terminal (-1 if none)
	umask int    // file creation mask
	pwd   string // working directory (replaces struct m_inode *pwd)
	/* Claude conversation state — replaces ldt[3] and tss */
	conv_id    string           // remote Claude conversation UUID
	prompt     string           // the prompt that started this process
	result     string           // output when process completes
	cancel     context.CancelFunc // replaces signal delivery to stop the process
	ctx        context.Context    // process context, cancelled on exit
	wait_queue chan int          // waitpid() blocks here (receives exit_code)
	mu sync.Mutex // protects concurrent access to this task
}
var (
	task [NR_TASKS]*task_struct
	current *task_struct
	last_pid int64
	jiffies int64
	startup_time int64
	mu sync.Mutex
)
func CURRENT_TIME() int64 {
	return startup_time + jiffies/100
}
func INIT_TASK() *task_struct {
	ctx, cancel := context.WithCancel(context.Background())
	return &task_struct{
		/* state etc */ state: TASK_RUNNING, counter: 15, priority: 15,
		/* signals */ signal: 0, blocked: 0,
		/* exit */ exit_code: 0,
		/* pid etc */ pid: 0, father: -1, pgrp: 0, session: 0, leader: 1,
		/* time */ utime: 0, stime: 0, cutime: 0, cstime: 0,
		start_time: 0,
		/* fs */ tty: -1, umask: 0022,
		/* claude */ conv_id: "", prompt: "", result: "",
		ctx: ctx, cancel: cancel,
		wait_queue: make(chan int, 1),
	}
}
func sched_init() {
	mu.Lock()
	defer mu.Unlock()
	startup_time = time.Now().Unix()
	jiffies = 0
	last_pid = 0
	for i := 0; i < NR_TASKS; i++ {
		task[i] = nil
	}
	task[0] = INIT_TASK()
	current = task[0]
}
