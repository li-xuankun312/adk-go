package kernel
import (
	"context"
	"fmt"
)
const EAGAIN = -11
func find_empty_process() int {
repeat:
	last_pid++
	if last_pid < 0 {
		last_pid = 1
	}
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == last_pid {
			goto repeat
		}
	}
	for i := 1; i < NR_TASKS; i++ {
		if task[i] == nil {
			return i
		}
	}
	return EAGAIN
}
func copy_process(prompt string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()
	nr := find_empty_process()
	if nr == EAGAIN {
		return 0, fmt.Errorf("copy_process: no free task slots (EAGAIN)")
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &task_struct{}
	if current != nil {
		*p = *current // copy parent's fields
	}
	p.state = TASK_UNINTERRUPTIBLE
	p.pid = last_pid
	if current != nil {
		p.father = current.pid
	} else {
		p.father = 0
	}
	p.counter = p.priority
	p.signal = 0
	p.alarm = 0
	p.leader = 0
	p.utime = 0
	p.stime = 0
	p.cutime = 0
	p.cstime = 0
	p.start_time = jiffies
	p.conv_id = "" // will be assigned when the conversation starts
	p.prompt = prompt
	p.result = ""
	p.ctx = ctx
	p.cancel = cancel
	p.wait_queue = make(chan int, 1)
	task[nr] = p
	p.state = TASK_RUNNING
	return last_pid, nil
}
