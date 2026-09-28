// kernel/exit.go
//
// Go port of linux-0.11 kernel/exit.c
// Contains release(), send_sig(), tell_father(), do_exit(),
// sys_exit(), and sys_waitpid().

package kernel

import (
	"fmt"
	"log"
)

// Signal constants matching linux-0.11
const (
	SIGHUP  = 1
	SIGINT  = 2
	SIGKILL = 9
	SIGPIPE = 13
	SIGCHLD = 17
	SIGSTOP = 19
)

// Wait options matching linux-0.11 sys/wait.h
const (
	WNOHANG    = 1
	WUNTRACED  = 2
)

// Error codes
const (
	ECHILD = -10
	EINTR  = -4
	EINVAL = -22
	EPERM  = -1
)

// release frees the specified task from the task table.
//
// Direct port of linux-0.11 release():
//
//	void release(struct task_struct * p)
//	{
//	    for (i=1 ; i<NR_TASKS ; i++)
//	        if (task[i]==p) {
//	            task[i]=NULL;
//	            free_page((long)p);
//	            schedule();
//	            return;
//	        }
//	    panic("trying to release non-existent task");
//	}
func release(p *task_struct) {
	if p == nil {
		return
	}
	for i := 1; i < NR_TASKS; i++ {
		if task[i] == p {
			task[i] = nil
			// free_page((long)p) — Go GC handles memory.
			// Cancel the process context to release resources.
			if p.cancel != nil {
				p.cancel()
			}
			return
		}
	}
	log.Printf("kernel: trying to release non-existent task (pid=%d)", p.pid)
}

// send_sig sends signal sig to process p.
//
// Direct port of linux-0.11 send_sig():
//
//	static inline int send_sig(long sig, struct task_struct * p, int priv)
//	{
//	    if (!p || sig<1 || sig>32) return -EINVAL;
//	    if (priv || (current->euid==p->euid) || suser())
//	        p->signal |= (1<<(sig-1));
//	    else return -EPERM;
//	    return 0;
//	}
func send_sig(sig int64, p *task_struct, priv int) int {
	if p == nil || sig < 1 || sig > 32 {
		return EINVAL
	}
	// In our model there are no uid checks; all signals are permitted.
	// This matches priv=1 (forced) behavior.
	_ = priv
	p.signal |= (1 << (sig - 1))
	return 0
}

// tell_father notifies the parent process (by pid) that a child has exited.
//
// Direct port of linux-0.11 tell_father():
//
//	static void tell_father(int pid)
//	{
//	    if (pid)
//	        for (i=0;i<NR_TASKS;i++) {
//	            if (!task[i]) continue;
//	            if (task[i]->pid != pid) continue;
//	            task[i]->signal |= (1<<(SIGCHLD-1));
//	            return;
//	        }
//	    printk("BAD BAD - no father found\n\r");
//	    release(current);
//	}
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

// do_exit terminates the current process.
//
// Port of linux-0.11 do_exit(). Strips out memory/fs/tty cleanup
// that doesn't apply to our model, keeps the process lifecycle logic.
//
//	int do_exit(long code)
//	{
//	    // free page tables...
//	    // reparent children to init (task[0])...
//	    for (i=0; i<NR_TASKS; i++)
//	        if (task[i] && task[i]->father == current->pid) {
//	            task[i]->father = 1;
//	            if (task[i]->state == TASK_ZOMBIE)
//	                (void) send_sig(SIGCHLD, task[1], 1);
//	        }
//	    // close files, release inodes...
//	    current->state = TASK_ZOMBIE;
//	    current->exit_code = code;
//	    tell_father(current->father);
//	    schedule();
//	    return (-1);
//	}
func do_exit(code int) int {
	// Must be called with mu held.

	// Reparent children to init process (task[0]), just like linux-0.11.
	// If any child is already ZOMBIE, signal init.
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

	// current->state = TASK_ZOMBIE;
	current.state = TASK_ZOMBIE

	// current->exit_code = code;
	current.exit_code = code

	// Cancel the process context (release goroutines, connections, etc.)
	if current.cancel != nil {
		current.cancel()
	}

	// Notify waiters via wait_queue channel
	select {
	case current.wait_queue <- code:
	default:
	}

	// tell_father(current->father);
	tell_father(current.father)

	// schedule() — in linux-0.11 this never returns.
	// In Go, we return and let the caller handle the goroutine exit.
	return -1
}

// sys_exit is the exit() system call.
//
// Direct port of linux-0.11:
//
//	int sys_exit(int error_code)
//	{
//	    return do_exit((error_code&0xff)<<8);
//	}
func sys_exit(error_code int) int {
	return do_exit((error_code & 0xff) << 8)
}

// sys_waitpid waits for a child process to exit.
//
// Port of linux-0.11 sys_waitpid(). In linux-0.11 this blocks by
// setting current->state = TASK_INTERRUPTIBLE and calling schedule().
// In Go, we block on the child's wait_queue channel instead.
//
// Parameters match linux-0.11:
//
//	pid > 0:  wait for child with this pid
//	pid = 0:  wait for any child in same process group
//	pid = -1: wait for any child
//	pid < -1: wait for any child in process group |pid|
//
// Returns (child_pid, exit_code, error).
func sys_waitpid(pid int64, options int) (int64, int, error) {
	mu.Lock()

	var found *task_struct
	flag := false

	// Scan task table for matching children, just like linux-0.11.
	// for(p = &LAST_TASK ; p > &FIRST_TASK ; --p)
	for i := NR_TASKS - 1; i > 0; i-- {
		p := task[i]
		if p == nil || p == current {
			continue
		}
		// if ((*p)->father != current->pid) continue;
		if p.father != current.pid {
			continue
		}

		// PID matching logic — exactly as in linux-0.11
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
		// pid == -1: match any child (no filter)

		switch p.state {
		case TASK_STOPPED:
			if options&WUNTRACED == 0 {
				continue
			}
			mu.Unlock()
			return p.pid, 0x7f, nil

		case TASK_ZOMBIE:
			// Accumulate child time into parent
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

	// No matching child at all
	if !flag {
		mu.Unlock()
		return 0, 0, fmt.Errorf("sys_waitpid: no children (ECHILD)")
	}

	// Child exists but not exited yet
	if options&WNOHANG != 0 {
		mu.Unlock()
		return 0, 0, nil
	}

	// Block until child exits.
	// In linux-0.11:
	//   current->state = TASK_INTERRUPTIBLE;
	//   schedule();
	// In Go: block on the child's wait_queue channel.
	waitCh := found.wait_queue
	mu.Unlock()

	code := <-waitCh

	// Child has exited. Clean it up.
	mu.Lock()
	childPid := found.pid
	current.cutime += found.utime
	current.cstime += found.stime
	release(found)
	mu.Unlock()

	return childPid, code, nil
}
