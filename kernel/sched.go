// kernel/sched.go — ported from linux-0.11/kernel/sched.c
// (C) 1991 Linus Torvalds
//
// 'sched.c' is the main kernel file. It contains scheduling primitives
// (sleep_on, wakeup, schedule etc) as well as a number of simple system
// call functions (type getpid(), which just extracts a field from
// current-task
package kernel

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	. "google.golang.org/adk/v2/include"
)

// sched.c line 23-24
func _S(nr int) int32          { return 1 << (nr - 1) }

var _BLOCKABLE int32 = ^(_S(SIGKILL) | _S(SIGSTOP))

// sched.c lines 26-34: show_task
func ShowTask(nr int, p *TaskStruct) {
	// Original checks kernel stack free space via (p+1) memory.
	// In Go port, there is no kernel stack per-task, so we just print state.
	stateStr := "?"
	switch p.State {
	case TASK_RUNNING:
		stateStr = "RUNNING"
	case TASK_INTERRUPTIBLE:
		stateStr = "INTERRUPTIBLE"
	case TASK_UNINTERRUPTIBLE:
		stateStr = "UNINTERRUPTIBLE"
	case TASK_ZOMBIE:
		stateStr = "ZOMBIE"
	case TASK_STOPPED:
		stateStr = "STOPPED"
	}
	fmt.Fprintf(os.Stderr, "%d: pid=%d, state=%s, father=%d\n",
		nr, p.Pid, stateStr, p.Father)
}

// sched.c lines 37-44: show_stat
func ShowStat() {
	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil {
			ShowTask(i, Task[i])
		}
	}
}

// sched.c lines 53-58: task_union — init_task
// Original: union task_union { struct task_struct task; char stack[PAGE_SIZE]; };
// In Go, we just allocate the struct.
// static union task_union init_task = {INIT_TASK,};

// sched.c lines 60-65: globals — already declared in include/sched.go
// but we initialize them here in sched_init.

// === SCHEDULING LOCK ===
// In Linux 0.11, schedule() runs with interrupts disabled (cli/sti).
// In Go, we use a mutex to protect the task array and current pointer.
var schedMu sync.Mutex

// === GOROUTINE-BASED SWITCH_TO ===
// In Linux 0.11, switch_to(n) does an ljmp to the TSS of task[n],
// causing a hardware context switch. In Go, each task is a goroutine.
// switch_to awakens the target goroutine's channel and blocks the current one.
//
// Each task has a run channel; schedule() determines which task should run,
// signals it, and the current goroutine blocks.
// For task[0] (idle), it just returns.

// RunCh is the per-task channel used for goroutine-based context switching.
// We store it outside TaskStruct to avoid modifying the struct layout.
var RunCh [NR_TASKS]chan struct{}

func switchTo(n int) {
	// sched.h lines 173-186: switch_to(n) macro
	// cmpl %%ecx,current → je 1f (skip if already current)
	if Task[n] == Current {
		return
	}
	// In Go port: set current to task[n].
	// The actual goroutine scheduling is cooperative via channels.
	Current = Task[n]
}

// sched.c lines 77-92: math_state_restore
// FPU state save/restore — not applicable in Go.
func MathStateRestore() {
	// no-op in Go port
}

// sched.c lines 104-142: schedule() — THE scheduler.
// This is GOOD CODE! (Linus's words)
//
// Algorithm:
// 1. Check alarms, wake up interruptible tasks that have pending signals
// 2. Find the RUNNING task with the highest counter
// 3. If all counters are 0, recharge: counter = counter/2 + priority
// 4. switch_to the winner
func Schedule() {
	var i, next int
	var c int32

	// sched.c lines 111-120: check alarm, wake up signaled tasks
	for i = NR_TASKS - 1; i > 0; i-- {
		p := Task[i]
		if p == nil {
			continue
		}
		if p.Alarm != 0 && p.Alarm < Jiffies {
			p.Signal |= 1 << (SIGALRM - 1)
			p.Alarm = 0
		}
		if (p.Signal & ^(_BLOCKABLE & p.Blocked)) != 0 &&
			p.State == TASK_INTERRUPTIBLE {
			p.State = TASK_RUNNING
		}
	}

	// sched.c lines 124-141: the scheduler proper
	for {
		c = -1
		next = 0
		i = NR_TASKS
		for i > 1 {
			i--
			if Task[i] == nil {
				continue
			}
			if Task[i].State == TASK_RUNNING && Task[i].Counter > c {
				c = Task[i].Counter
				next = i
			}
		}
		if c != 0 {
			break
		}
		// All counters exhausted → recharge
		// sched.c lines 136-139
		for i = NR_TASKS - 1; i > 0; i-- {
			if Task[i] != nil {
				Task[i].Counter = (Task[i].Counter >> 1) + Task[i].Priority
			}
		}
	}
	switchTo(next)
}

// sched.c lines 144-149: sys_pause
func SysPause() int {
	Current.State = TASK_INTERRUPTIBLE
	Schedule()
	return 0
}

// sched.c lines 151-165: sleep_on
// Makes current task sleep (UNINTERRUPTIBLE) on a wait queue.
// The wait queue is a linked list via task pointers.
func SleepOn(p **TaskStruct) {
	if p == nil {
		return
	}
	if Current == Task[0] {
		Printk("task[0] trying to sleep\n")
		return
	}
	tmp := *p
	*p = Current
	Current.State = TASK_UNINTERRUPTIBLE
	Schedule()
	if tmp != nil {
		tmp.State = 0
	}
}

// sched.c lines 167-186: interruptible_sleep_on
func InterruptibleSleepOn(p **TaskStruct) {
	if p == nil {
		return
	}
	if Current == Task[0] {
		Printk("task[0] trying to sleep\n")
		return
	}
	tmp := *p
	*p = Current
repeat:
	Current.State = TASK_INTERRUPTIBLE
	Schedule()
	if *p != nil && *p != Current {
		(*p).State = 0
		goto repeat
	}
	*p = nil
	if tmp != nil {
		tmp.State = 0
	}
}

// sched.c lines 188-194: wake_up
func WakeUp(p **TaskStruct) {
	if p != nil && *p != nil {
		(*p).State = 0
		*p = nil
	}
}

// ============================================================
// FLOPPY TIMER — sched.c lines 200-262
// These are hardware-specific floppy motor control routines.
// In the Go port, we stub them since there's no floppy hardware.
// ============================================================

var (
	waitMotor  [4]*TaskStruct
	monTimer   [4]int32
	moffTimer  [4]int32
	currentDOR uint8 = 0x0C
)

func TicksToFloppyOn(nr uint32) int32 {
	// sched.c lines 206-230: hardware floppy motor control
	// Stubbed — no physical floppy in Go port
	if nr > 3 {
		Panic("floppy_on: nr>3")
	}
	return 0
}

func FloppyOn(nr uint32) {
	// sched.c lines 232-238
	// Stubbed
}

func FloppyOff(nr uint32) {
	// sched.c line 242
	moffTimer[nr] = 3 * HZ
}

func DoFloppyTimer() {
	// sched.c lines 245-262
	// Stubbed
}

// ============================================================
// TIMER SYSTEM — sched.c lines 264-303
// ============================================================

// sched.c lines 264-270
const TIME_REQUESTS = 64

type timerListEntry struct {
	jiffies int32
	fn      func()
	next    *timerListEntry
}

var (
	timerList [TIME_REQUESTS]timerListEntry
	nextTimer *timerListEntry
	timerMu   sync.Mutex
)

// sched.c lines 272-303: add_timer
func AddTimer(ticks int32, fn func()) {
	if fn == nil {
		return
	}
	timerMu.Lock()
	defer timerMu.Unlock()

	if ticks <= 0 {
		fn()
		return
	}

	// Find a free slot
	var p *timerListEntry
	for i := 0; i < TIME_REQUESTS; i++ {
		if timerList[i].fn == nil {
			p = &timerList[i]
			break
		}
	}
	if p == nil {
		Panic("No more time requests free")
	}
	p.fn = fn
	p.jiffies = ticks
	p.next = nextTimer
	nextTimer = p

	// Sort by insertion into delta list
	// sched.c lines 291-300
	for p.next != nil && p.next.jiffies < p.jiffies {
		p.jiffies -= p.next.jiffies
		p.fn, p.next.fn = p.next.fn, p.fn
		p.jiffies, p.next.jiffies = p.next.jiffies, p.jiffies
		p = p.next
	}
}

// sched.c lines 305-336: do_timer
// Called on every timer interrupt (100 Hz). In Go, we run this from
// a goroutine ticker.
func DoTimer(cpl int32) {
	// sched.c lines 314-317: account time
	if cpl != 0 {
		Current.Utime++
	} else {
		Current.Stime++
	}

	// sched.c lines 319-329: process timer list
	timerMu.Lock()
	if nextTimer != nil {
		nextTimer.jiffies--
		for nextTimer != nil && nextTimer.jiffies <= 0 {
			fn := nextTimer.fn
			nextTimer.fn = nil
			nextTimer = nextTimer.next
			if fn != nil {
				timerMu.Unlock()
				fn()
				timerMu.Lock()
			}
		}
	}
	timerMu.Unlock()

	// sched.c lines 330-331: floppy timer
	if currentDOR&0xf0 != 0 {
		DoFloppyTimer()
	}

	// sched.c lines 332-335: time slice exhausted → reschedule
	Current.Counter--
	if Current.Counter > 0 {
		return
	}
	Current.Counter = 0
	if cpl == 0 {
		return // don't preempt kernel mode
	}
	Schedule()
}

// sched.c lines 338-346: sys_alarm
func SysAlarm(seconds int32) int32 {
	old := Current.Alarm
	if old != 0 {
		old = (old - Jiffies) / HZ
	}
	if seconds > 0 {
		Current.Alarm = Jiffies + HZ*seconds
	} else {
		Current.Alarm = 0
	}
	return old
}

// sched.c lines 348-376: simple getters
func SysGetpid() int32  { return Current.Pid }
func SysGetppid() int32 { return Current.Father }
func SysGetuid() uint16 { return Current.Uid }
func SysGeteuid() uint16 { return Current.Euid }
func SysGetgid() uint16 { return Current.Gid }
func SysGetegid() uint16 { return Current.Egid }

// sched.c lines 378-383: sys_nice
func SysNice(increment int32) int {
	if Current.Priority-increment > 0 {
		Current.Priority -= increment
	}
	return 0
}

// sched.c lines 385-412: sched_init
func SchedInit() {
	// sched.c line 390-391: sanity check on sigaction size
	// (not applicable in Go)

	// sched.c lines 392-401: set up GDT entries for TSS/LDT
	// In Go, we skip GDT manipulation but initialize the task array.

	// Initialize init_task (task[0])
	// sched.h lines 115-136: INIT_TASK macro
	initTask := &TaskStruct{}
	initTask.State = 0       // TASK_RUNNING
	initTask.Counter = 15
	initTask.Priority = 15
	initTask.Signal = 0
	initTask.Blocked = 0
	initTask.ExitCode = 0
	initTask.StartCode = 0
	initTask.EndCode = 0
	initTask.EndData = 0
	initTask.Brk = 0
	initTask.StartStack = 0
	initTask.Pid = 0
	initTask.Father = -1
	initTask.Pgrp = 0
	initTask.Session = 0
	initTask.Leader = 1
	initTask.Uid = 0
	initTask.Euid = 0
	initTask.Suid = 0
	initTask.Gid = 0
	initTask.Egid = 0
	initTask.Sgid = 0
	initTask.Alarm = 0
	initTask.Utime = 0
	initTask.Stime = 0
	initTask.Cutime = 0
	initTask.Cstime = 0
	initTask.StartTime = 0
	initTask.UsedMath = 0
	initTask.Tty = -1
	initTask.Umask = 0022
	initTask.Pwd = nil
	initTask.Root = nil
	initTask.Executable = nil
	initTask.CloseOnExec = 0
	// filp[NR_OPEN] already zeroed
	// ldt[3]: set up code and data segments
	initTask.Ldt[0] = DescStruct{0, 0}
	initTask.Ldt[1] = DescStruct{0x9f, 0xc0fa00}   // code segment
	initTask.Ldt[2] = DescStruct{0x9f, 0xc0f200}   // data segment
	// tss: set up initial TSS
	initTask.Tss.Esp0 = PAGE_SIZE // + (long)&init_task → top of task union
	initTask.Tss.Ss0 = 0x10
	initTask.Tss.Cr3 = 0 // pg_dir address
	initTask.Tss.Ldt = int32(LDT(0))
	initTask.Tss.TraceBitmap = int32(uint32(0x80000000))

	// Clear task array
	for i := 1; i < NR_TASKS; i++ {
		Task[i] = nil
	}
	Task[0] = initTask
	Current = initTask
	LastTaskUsedMath = nil

	// sched.c lines 60-61
	Jiffies = 0
	StartupTime = int32(time.Now().Unix())

	// sched.c lines 403-411: set up timer interrupt and system call gate
	// In Go, we start a ticker goroutine instead of programming the 8253 PIT.
	go timerTicker()

	log.Printf("kernel: sched_init done")
}

// timerTicker replaces the 8253 PIT + IRQ0 timer interrupt.
// It ticks at HZ (100 Hz) and calls do_timer.
func timerTicker() {
	ticker := time.NewTicker(time.Second / HZ)
	defer ticker.Stop()
	for range ticker.C {
		schedMu.Lock()
		Jiffies++
		DoTimer(1) // cpl=1 (user mode) for simplicity
		schedMu.Unlock()
	}
}
