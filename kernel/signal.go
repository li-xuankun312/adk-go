package kernel
func sys_kill(pid int64, sig int64) int {
	mu.Lock()
	defer mu.Unlock()
	retval := 0
	if pid == 0 {
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil && task[i].pgrp == current.pid {
				if err := send_sig(sig, task[i], 1); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	} else if pid > 0 {
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil && task[i].pid == pid {
				if err := send_sig(sig, task[i], 0); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	} else if pid == -1 {
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil {
				if err := send_sig(sig, task[i], 0); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	} else {
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil && task[i].pgrp == -pid {
				if err := send_sig(sig, task[i], 0); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	}
	return retval
}
func deliverSignal(p *task_struct, sig int64) {
	if p == nil {
		return
	}
	switch sig {
	case SIGKILL, SIGINT:
		if p.cancel != nil {
			p.cancel()
		}
		select {
		case p.wait_queue <- -1:
		default:
		}
	}
}
func kill_session() {
	for i := NR_TASKS - 1; i > 0; i-- {
		if task[i] != nil && task[i].session == current.session {
			task[i].signal |= 1 << (SIGHUP - 1)
			deliverSignal(task[i], SIGHUP)
		}
	}
}
