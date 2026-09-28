package kernel
import (
	"fmt"
	"log"
)
const (
	SIGHUP  = 1
	SIGINT  = 2
	SIGKILL = 9
	SIGPIPE = 13
	SIGCHLD = 17
	SIGSTOP = 19
)
const (
	WNOHANG    = 1
	WUNTRACED  = 2
)
const (
	ECHILD = -10
	EINTR  = -4
	EINVAL = -22
	EPERM  = -1
)
func release(p *task_struct) {
	if p == nil {
		return
	}
	for i := 1; i < NR_TASKS; i++ {
		if task[i] == p {
			task[i] = nil
			if p.cancel != nil {
				p.cancel()
			}
			return
		}
	}
	log.Printf("kernel: trying to release non-existent task (pid=%d)", p.pid)
}
func send_sig(sig int64, p *task_struct, priv int) int {
	if p == nil || sig < 1 || sig > 32 {
		return EINVAL
	}
	_ = priv
	p.signal |= (1 << (sig - 1))
	return 0
}
func tell_father(pid int64) {
	if pid != 0 {
		for i := 0; i < NR_TASKS; i++ {
			if task[i] == nil {
				continue
			}
			if task[i].pid != pid {
				continue
			}
			task[i].signal |= (1 << (SIGCHLD - 1))
			return
		}
	}
	log.Printf("kernel: BAD BAD - no father found for pid %d", pid)
	release(current)
}
func do_exit(code int) int {
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].father == current.pid {
			task[i].father = 0 // reparent to init (pid 0 in our model)
			if task[i].state == TASK_ZOMBIE {
				if task[0] != nil {
					send_sig(SIGCHLD, task[0], 1)
				}
			}
		}
	}
	current.state = TASK_ZOMBIE
	current.exit_code = code
	if current.cancel != nil {
		current.cancel()
	}
	select {
	case current.wait_queue <- code:
	default:
	}
	tell_father(current.father)
	return -1
}
func sys_exit(error_code int) int {
	return do_exit((error_code & 0xff) << 8)
}
func sys_waitpid(pid int64, options int) (int64, int, error) {
	mu.Lock()
	var found *task_struct
	flag := false
	for i := NR_TASKS - 1; i > 0; i-- {
		p := task[i]
		if p == nil || p == current {
			continue
		}
		if p.father != current.pid {
			continue
		}
		if pid > 0 {
			if p.pid != pid {
				continue
			}
		} else if pid == 0 {
			if p.pgrp != current.pgrp {
				continue
			}
		} else if pid != -1 {
			if p.pgrp != -pid {
				continue
			}
		}
		switch p.state {
		case TASK_STOPPED:
			if options&WUNTRACED == 0 {
				continue
			}
			mu.Unlock()
			return p.pid, 0x7f, nil
		case TASK_ZOMBIE:
			current.cutime += p.utime
			current.cstime += p.stime
			childPid := p.pid
			code := p.exit_code
			release(p)
			mu.Unlock()
			return childPid, code, nil
		default:
			flag = true
			found = p
			continue
		}
	}
	if !flag {
		mu.Unlock()
		return 0, 0, fmt.Errorf("sys_waitpid: no children (ECHILD)")
	}
	if options&WNOHANG != 0 {
		mu.Unlock()
		return 0, 0, nil
	}
	waitCh := found.wait_queue
	mu.Unlock()
	code := <-waitCh
	mu.Lock()
	childPid := found.pid
	current.cutime += found.utime
	current.cstime += found.stime
	release(found)
	mu.Unlock()
	return childPid, code, nil
}
