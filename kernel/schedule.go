package kernel
import (
	"fmt"
	"log"
)
func _S(nr int) int64 {
	return 1 << (nr - 1)
}
var _BLOCKABLE int64 = ^(_S(SIGKILL) | _S(SIGSTOP))
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
func schedule() {
	for i := NR_TASKS - 1; i > 0; i-- {
		p := task[i]
		if p == nil {
			continue
		}
		if p.alarm != 0 && p.alarm < jiffies {
			p.signal |= _S(14) // SIGALRM = 14
			p.alarm = 0
		}
		if (p.signal & ^(_BLOCKABLE & p.blocked)) != 0 {
			if p.state == TASK_INTERRUPTIBLE {
				p.state = TASK_RUNNING
			}
		}
	}
}
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
	if tmp != nil {
		tmp.state = TASK_RUNNING
	}
}
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
func wake_up(p **task_struct) {
	if p != nil && *p != nil {
		(*p).state = TASK_RUNNING
		*p = nil
	}
}
