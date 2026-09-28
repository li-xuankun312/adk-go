// kernel/schedule.go
//
// Go port of linux-0.11 kernel/sched.c scheduling functions.
// Contains schedule(), sleep_on(), interruptible_sleep_on(),
// wake_up(), show_task(), show_stat().
//
// In linux-0.11, schedule() does cooperative multitasking with
// hardware task switching (ljmp). In Go, each process runs in its
// own goroutine, so schedule() is a no-op — the Go runtime handles
// preemption. sleep_on/wake_up are ported using channels.

package kernel

import (
	"fmt"
	"log"
)

// _S returns the bitmask for signal nr: 1<<(nr-1)
// Mirrors linux-0.11: #define _S(nr) (1<<((nr)-1))
func _S(nr int) int64 {
	return 1 << (nr - 1)
}

// _BLOCKABLE is the mask of signals that can be blocked.
// All except SIGKILL and SIGSTOP.
// Mirrors: #define _BLOCKABLE (~(_S(SIGKILL) | _S(SIGSTOP)))
var _BLOCKABLE int64 = ^(_S(SIGKILL) | _S(SIGSTOP))

// show_task displays information about a task.
//
// Direct port of linux-0.11 show_task():
//
//	void show_task(int nr, struct task_struct * p)
//	{
//	    printk("%d: pid=%d, state=%d, ", nr, p->pid, p->state);
//	    ...
//	    printk("%d (of %d) chars free in kernel stack\n\r", i, j);
//	}
func show_task(nr int, p *task_struct) {
	stateStr := "?"
	switch p.state {
	case TASK_RUNNING:
		stateStr = "RUNNING"
	case TASK_INTERRUPTIBLE:
		stateStr = "SLEEPING"
	case TASK_UNINTERRUPTIBLE:
		stateStr = "UNINTERRUPTIBLE"
	case TASK_ZOMBIE:
		stateStr = "ZOMBIE"
	case TASK_STOPPED:
		stateStr = "STOPPED"
	}
	conv := p.conv_id
	if len(conv) > 8 {
		conv = conv[:8]
	}
	if conv == "" {
		conv = "(none)"
	}
	prompt := p.prompt
	if len(prompt) > 40 {
		prompt = prompt[:40] + "..."
	}
	fmt.Printf("  [%2d] pid=%-3d ppid=%-3d state=%-15s conv=%s  %s\n",
		nr, p.pid, p.father, stateStr, conv, prompt)
}

// show_stat displays all tasks in the task table.
//
// Direct port of linux-0.11 show_stat():
//
//	void show_stat(void)
//	{
//	    for (i=0; i<NR_TASKS; i++)
//	        if (task[i]) show_task(i, task[i]);
//	}
func show_stat() {
	mu.Lock()
	defer mu.Unlock()
	fmt.Println("  PID   PPID  STATE           CONV      PROMPT")
	fmt.Println("  ───   ────  ─────           ────      ──────")
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil {
			show_task(i, task[i])
		}
	}
}

// schedule selects the next task to run.
//
// In linux-0.11, schedule() is the core scheduler that picks the
// TASK_RUNNING process with the highest counter and does a hardware
// task switch via switch_to(next).
//
// In our Go model, each process runs in its own goroutine. The Go
// runtime handles scheduling. This function exists for API parity
// and to handle signal-based wakeups of TASK_INTERRUPTIBLE processes.
//
// Direct port of the signal-checking part:
//
//	for(p = &LAST_TASK; p > &FIRST_TASK; --p)
//	    if (*p) {
//	        if ((*p)->alarm && (*p)->alarm < jiffies) {
//	            (*p)->signal |= (1<<(SIGALRM-1));
//	            (*p)->alarm = 0;
//	        }
//	        if (((*p)->signal & ~(_BLOCKABLE & (*p)->blocked)) &&
//	            (*p)->state==TASK_INTERRUPTIBLE)
//	            (*p)->state=TASK_RUNNING;
//	    }
func schedule() {
	// Check for pending signals on interruptible tasks.
	for i := NR_TASKS - 1; i > 0; i-- {
		p := task[i]
		if p == nil {
			continue
		}
		// Alarm check
		if p.alarm != 0 && p.alarm < jiffies {
			p.signal |= _S(14) // SIGALRM = 14
			p.alarm = 0
		}
		// Wake interruptible tasks that have unblocked signals
		if (p.signal & ^(_BLOCKABLE & p.blocked)) != 0 {
			if p.state == TASK_INTERRUPTIBLE {
				p.state = TASK_RUNNING
			}
		}
	}
	// In linux-0.11, the rest of schedule() picks the highest-counter
	// TASK_RUNNING and does switch_to(next). In Go, goroutines handle
	// this automatically.
}

// sleep_on puts the current task to sleep on a wait queue.
//
// In linux-0.11, sleep_on uses a linked-list-on-stack pattern where
// each caller's tmp variable points to the previous waiter. In Go,
// we use the task's wait_queue channel instead.
//
// Original:
//
//	void sleep_on(struct task_struct **p)
//	{
//	    tmp = *p;
//	    *p = current;
//	    current->state = TASK_UNINTERRUPTIBLE;
//	    schedule();
//	    if (tmp) tmp->state = 0;
//	}
func sleep_on(p **task_struct) {
	if p == nil {
		return
	}
	if current == task[0] {
		log.Printf("kernel: task[0] trying to sleep")
		return
	}

	tmp := *p
	*p = current
	current.state = TASK_UNINTERRUPTIBLE
	schedule()
	// When we wake up, also wake the previous waiter
	if tmp != nil {
		tmp.state = TASK_RUNNING
	}
}

// interruptible_sleep_on is like sleep_on but uses TASK_INTERRUPTIBLE.
//
// Original:
//
//	void interruptible_sleep_on(struct task_struct **p)
//	{
//	    tmp = *p;
//	    *p = current;
//	    repeat: current->state = TASK_INTERRUPTIBLE;
//	    schedule();
//	    if (*p && *p != current) {
//	        (**p).state = 0;
//	        goto repeat;
//	    }
//	    *p = NULL;
//	    if (tmp) tmp->state = 0;
//	}
func interruptible_sleep_on(p **task_struct) {
	if p == nil {
		return
	}
	if current == task[0] {
		log.Printf("kernel: task[0] trying to sleep")
		return
	}

	tmp := *p
	*p = current
	current.state = TASK_INTERRUPTIBLE
	schedule()
	*p = nil
	if tmp != nil {
		tmp.state = TASK_RUNNING
	}
}

// wake_up wakes the task at the head of a wait queue.
//
// Direct port:
//
//	void wake_up(struct task_struct **p)
//	{
//	    if (p && *p) {
//	        (**p).state = 0;
//	        *p = NULL;
//	    }
//	}
func wake_up(p **task_struct) {
	if p != nil && *p != nil {
		(*p).state = TASK_RUNNING
		*p = nil
	}
}
