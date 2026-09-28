// kernel/fork.go
//
// Go port of linux-0.11 kernel/fork.c
// Contains find_empty_process() and copy_process().

package kernel

import (
	"context"
	"fmt"
)

// EAGAIN is returned when the task table is full.
const EAGAIN = -11

// find_empty_process finds an unused PID and an empty slot in the task table.
// Returns the task table index (nr), or EAGAIN if no slot is available.
//
// Direct port of linux-0.11 find_empty_process():
//
//	repeat:
//	    if ((++last_pid)<0) last_pid=1;
//	    for(i=0 ; i<NR_TASKS ; i++)
//	        if (task[i] && task[i]->pid == last_pid) goto repeat;
//	for(i=1 ; i<NR_TASKS ; i++)
//	    if (!task[i])
//	        return i;
//	return -EAGAIN;
func find_empty_process() int {
	// Must be called with mu held.

	// Increment last_pid, wrapping from 1 if it overflows.
repeat:
	last_pid++
	if last_pid < 0 {
		last_pid = 1
	}
	// Check that last_pid is not already in use by any task.
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == last_pid {
			goto repeat
		}
	}
	// Find an empty slot in the task table (task[0] is init, skip it).
	for i := 1; i < NR_TASKS; i++ {
		if task[i] == nil {
			return i
		}
	}
	return EAGAIN
}

// copy_process creates a new process by copying the current process.
// This is the Go equivalent of linux-0.11's copy_process().
//
// In linux-0.11, copy_process copies registers (tss), page tables,
// and open files. In our model:
//   - tss/registers → Claude conversation state (new conv_id, fresh ctx)
//   - page tables   → shared filesystem (inherited pwd)
//   - open files    → not applicable
//
// Parameters:
//   - prompt: the initial prompt for the child process (replaces eip)
//
// Returns the new process's PID, or a negative error code.
//
// Original signature (linux-0.11):
//
//	int copy_process(int nr, long ebp, long edi, long esi, long gs, long none,
//	    long ebx, long ecx, long edx,
//	    long fs, long es, long ds,
//	    long eip, long cs, long eflags, long esp, long ss)
func copy_process(prompt string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()

	// find_empty_process — allocate a PID and find a free task slot.
	nr := find_empty_process()
	if nr == EAGAIN {
		return 0, fmt.Errorf("copy_process: no free task slots (EAGAIN)")
	}

	// p = (struct task_struct *) get_free_page();
	// *p = *current;  /* NOTE! this doesn't copy the supervisor stack */
	ctx, cancel := context.WithCancel(context.Background())
	p := &task_struct{}
	if current != nil {
		*p = *current // copy parent's fields
	}

	// Overwrite fields that differ for the child process.
	// Mirrors linux-0.11 copy_process() lines 136-145.

	// p->state = TASK_UNINTERRUPTIBLE;
	p.state = TASK_UNINTERRUPTIBLE

	// p->pid = last_pid;
	p.pid = last_pid

	// p->father = current->pid;
	if current != nil {
		p.father = current.pid
	} else {
		p.father = 0
	}

	// p->counter = p->priority;
	p.counter = p.priority

	// p->signal = 0;
	p.signal = 0

	// p->alarm = 0;
	p.alarm = 0

	// p->leader = 0;  /* process leadership doesn't inherit */
	p.leader = 0

	// p->utime = p->stime = 0;
	p.utime = 0
	p.stime = 0

	// p->cutime = p->cstime = 0;
	p.cutime = 0
	p.cstime = 0

	// p->start_time = jiffies;
	p.start_time = jiffies

	// --- TSS equivalent: Claude conversation state ---
	// In linux-0.11 this sets up tss.eip, tss.esp, tss.eax=0, etc.
	// For us: new conversation, fresh context, the prompt is our "eip".
	p.conv_id = "" // will be assigned when the conversation starts
	p.prompt = prompt
	p.result = ""
	p.ctx = ctx
	p.cancel = cancel
	p.wait_queue = make(chan int, 1)

	// --- copy_mem equivalent ---
	// In linux-0.11 this copies page tables (cow). For us: inherit pwd.
	// pwd is already copied from *current above.

	// --- File reference count increment ---
	// In linux-0.11:
	//   for (i=0; i<NR_OPEN; i++)
	//       if ((f=p->filp[i])) f->f_count++;
	//   if (current->pwd) current->pwd->i_count++;
	// For us: pwd is a string (copied by value), no refcount needed.

	// task[nr] = p;
	task[nr] = p

	// p->state = TASK_RUNNING;  /* do this last, just in case */
	p.state = TASK_RUNNING

	// return last_pid;
	return last_pid, nil
}
