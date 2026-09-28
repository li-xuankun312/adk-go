// kernel/signal.go
//
// Go port of linux-0.11 signal handling from kernel/exit.c.
// Contains sys_kill() and kill_session().

package kernel

// sys_kill sends signal sig to process(es) identified by pid.
//
// Direct port of linux-0.11 sys_kill():
//
//	int sys_kill(int pid, int sig)
//	{
//	    if (!pid) while (--p > &FIRST_TASK)
//	        if (*p && (*p)->pgrp == current->pid)
//	            if ((err=send_sig(sig,*p,1))) retval = err;
//	    else if (pid>0) while (--p > &FIRST_TASK)
//	        if (*p && (*p)->pid == pid)
//	            if ((err=send_sig(sig,*p,0))) retval = err;
//	    else if (pid == -1) while (--p > &FIRST_TASK)
//	        if ((err = send_sig(sig,*p,0))) retval = err;
//	    else while (--p > &FIRST_TASK)
//	        if (*p && (*p)->pgrp == -pid)
//	            if ((err = send_sig(sig,*p,0))) retval = err;
//	    return retval;
//	}
//
// Additionally, for SIGKILL and SIGINT, we cancel the process context
// to actually interrupt any running goroutine (the Go equivalent of
// delivering a signal to a running process).
func sys_kill(pid int64, sig int64) int {
	mu.Lock()
	defer mu.Unlock()

	retval := 0

	if pid == 0 {
		// Signal all processes in current's process group
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil && task[i].pgrp == current.pid {
				if err := send_sig(sig, task[i], 1); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	} else if pid > 0 {
		// Signal specific process
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil && task[i].pid == pid {
				if err := send_sig(sig, task[i], 0); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	} else if pid == -1 {
		// Signal all processes (except init)
		for i := NR_TASKS - 1; i > 0; i-- {
			if task[i] != nil {
				if err := send_sig(sig, task[i], 0); err != 0 {
					retval = err
				}
				deliverSignal(task[i], sig)
			}
		}
	} else {
		// Signal all processes in process group |pid|
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

// deliverSignal performs the actual Go-level signal delivery.
// In linux-0.11, the kernel delivers signals by modifying the process's
// return path on the stack. In Go, we cancel the context for fatal signals.
func deliverSignal(p *task_struct, sig int64) {
	if p == nil {
		return
	}
	switch sig {
	case SIGKILL, SIGINT:
		// Cancel the process context, which will interrupt any
		// blocking operation (HTTP request, channel wait, etc.)
		if p.cancel != nil {
			p.cancel()
		}
		// Wake up anyone waiting on this process
		select {
		case p.wait_queue <- -1:
		default:
		}
	}
}

// kill_session terminates all processes in the current session.
//
// Direct port of linux-0.11 kill_session():
//
//	static void kill_session(void)
//	{
//	    struct task_struct **p = NR_TASKS + task;
//	    while (--p > &FIRST_TASK) {
//	        if (*p && (*p)->session == current->session)
//	            (*p)->signal |= 1<<(SIGHUP-1);
//	    }
//	}
func kill_session() {
	for i := NR_TASKS - 1; i > 0; i-- {
		if task[i] != nil && task[i].session == current.session {
			task[i].signal |= 1 << (SIGHUP - 1)
			deliverSignal(task[i], SIGHUP)
		}
	}
}
