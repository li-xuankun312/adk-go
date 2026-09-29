// kernel/exit.go — ported from linux-0.11/kernel/exit.c
// (C) 1991 Linus Torvalds
package kernel

import (
	. "google.golang.org/adk/v2/include"
)

// Error codes from include/errno.go via dot import

// Waitpid options from sys/wait.h
const (
	WNOHANG   = 1
	WUNTRACED = 2
)

// exit.c lines 19-33: release
func Release(p *TaskStruct) {
	if p == nil {
		return
	}
	for i := 1; i < NR_TASKS; i++ {
		if Task[i] == p {
			Task[i] = nil
			// In C: free_page((long)p); — in Go, GC handles it
			Schedule()
			return
		}
	}
	Panic("trying to release non-existent task")
}

// exit.c lines 35-44: send_sig
func SendSig(sig int32, p *TaskStruct, priv int) int {
	if p == nil || sig < 1 || sig > 32 {
		return -EINVAL
	}
	// exit.c line 39: permission check
	// priv || (current->euid==p->euid) || suser()
	if priv != 0 || Current.Euid == p.Euid || suser() {
		p.Signal |= (1 << (sig - 1))
	} else {
		return -EPERM
	}
	return 0
}

// suser() — check if current process is superuser
func suser() bool {
	return Current.Euid == 0
}

// exit.c lines 46-54: kill_session
func KillSession() {
	// struct task_struct **p = NR_TASKS + task;
	// while (--p > &FIRST_TASK)
	for i := NR_TASKS - 1; i > 0; i-- {
		if Task[i] != nil && Task[i].Session == Current.Session {
			Task[i].Signal |= 1 << (SIGHUP - 1)
		}
	}
}

// exit.c lines 60-81: sys_kill
func SysKill(pid int32, sig int32) int {
	retval := 0

	if pid == 0 {
		// kill process group of current
		for i := NR_TASKS - 1; i > 0; i-- {
			if Task[i] != nil && Task[i].Pgrp == Current.Pid {
				if err := SendSig(sig, Task[i], 1); err != 0 {
					retval = err
				}
			}
		}
	} else if pid > 0 {
		// kill specific pid
		for i := NR_TASKS - 1; i > 0; i-- {
			if Task[i] != nil && Task[i].Pid == pid {
				if err := SendSig(sig, Task[i], 0); err != 0 {
					retval = err
				}
			}
		}
	} else if pid == -1 {
		// kill all (except task 0)
		for i := NR_TASKS - 1; i > 0; i-- {
			if err := SendSig(sig, Task[i], 0); err != 0 {
				retval = err
			}
		}
	} else {
		// kill process group -pid
		for i := NR_TASKS - 1; i > 0; i-- {
			if Task[i] != nil && Task[i].Pgrp == -pid {
				if err := SendSig(sig, Task[i], 0); err != 0 {
					retval = err
				}
			}
		}
	}
	return retval
}

// exit.c lines 83-100: tell_father
func TellFather(pid int32) {
	if pid != 0 {
		for i := 0; i < NR_TASKS; i++ {
			if Task[i] == nil {
				continue
			}
			if Task[i].Pid != pid {
				continue
			}
			Task[i].Signal |= (1 << (SIGCHLD - 1))
			return
		}
	}
	// if we don't find any fathers, we just release ourselves
	// This is not really OK. Must change it to make father 1
	Printk("BAD BAD - no father found\n")
	Release(Current)
}

// exit.c lines 102-134: do_exit
func DoExit(code int32) int {
	// exit.c lines 105-106: free page tables
	if freePageTablesFn != nil {
		freePageTablesFn(GetBase(Current.Ldt[1]),
			int32(GetLimit(Current.Ldt[1])))
		freePageTablesFn(GetBase(Current.Ldt[2]),
			int32(GetLimit(Current.Ldt[2])))
	}

	// exit.c lines 107-113: reparent children to task[1] (init)
	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil && Task[i].Father == Current.Pid {
			Task[i].Father = 1
			if Task[i].State == TASK_ZOMBIE {
				// assumption task[1] is always init
				if Task[1] != nil {
					SendSig(SIGCHLD, Task[1], 1)
				}
			}
		}
	}

	// exit.c lines 114-116: close all open files
	for i := 0; i < NR_OPEN; i++ {
		if Current.Filp[i] != nil {
			SysClose(int32(i))
		}
	}

	// exit.c lines 117-122: release inodes
	if iputFn != nil {
		if Current.Pwd != nil {
			iputFn(Current.Pwd)
		}
		Current.Pwd = nil
		if Current.Root != nil {
			iputFn(Current.Root)
		}
		Current.Root = nil
		if Current.Executable != nil {
			iputFn(Current.Executable)
		}
		Current.Executable = nil
	}

	// exit.c lines 123-124: release controlling terminal
	if Current.Leader != 0 && Current.Tty >= 0 {
		// tty_table[current->tty].pgrp = 0;
		// Handled by tty subsystem
	}

	// exit.c lines 125-126: clear math state
	if LastTaskUsedMath == Current {
		LastTaskUsedMath = nil
	}

	// exit.c lines 127-128: kill session if leader
	if Current.Leader != 0 {
		KillSession()
	}

	// exit.c lines 129-131
	Current.State = TASK_ZOMBIE
	Current.ExitCode = code
	TellFather(Current.Father)

	// exit.c line 132
	Schedule()
	return -1 // just to suppress warnings
}

// iputFn is set by fs package to avoid circular import
var iputFn func(*MInode)

func SetIput(fn func(*MInode)) { iputFn = fn }

// SysClose is declared here (implemented in fs package)
// For now, a stub that clears the file pointer
func SysClose(fd int32) int {
	if fd < 0 || fd >= NR_OPEN {
		return -1
	}
	if Current.Filp[fd] != nil {
		Current.Filp[fd].FCount--
		if Current.Filp[fd].FCount == 0 {
			// In full implementation, this releases the inode
		}
		Current.Filp[fd] = nil
	}
	return 0
}

// exit.c lines 136-138: sys_exit
func SysExit(errorCode int32) int {
	return DoExit((errorCode & 0xff) << 8)
}

// exit.c lines 141-194: sys_waitpid
func SysWaitpid(pid int32, statAddr *int32, options int32) int32 {
	var flag int32
	// exit.c line 146: verify_area(stat_addr,4)
	// In Go, statAddr is a Go pointer — no need to verify.

repeat:
	flag = 0
	// exit.c lines 149-182
	for i := NR_TASKS - 1; i > 0; i-- {
		p := Task[i]
		if p == nil || p == Current {
			continue
		}
		if p.Father != Current.Pid {
			continue
		}
		// exit.c lines 154-163: pid matching
		if pid > 0 {
			if p.Pid != pid {
				continue
			}
		} else if pid == 0 {
			if p.Pgrp != Current.Pgrp {
				continue
			}
		} else if pid != -1 {
			if p.Pgrp != -pid {
				continue
			}
		}
		// exit.c lines 164-181: state check
		switch p.State {
		case TASK_STOPPED:
			if options&WUNTRACED == 0 {
				continue
			}
			// put_fs_long(0x7f,stat_addr)
			if statAddr != nil {
				*statAddr = 0x7f
			}
			return p.Pid

		case TASK_ZOMBIE:
			// exit.c lines 171-177
			Current.Cutime += p.Utime
			Current.Cstime += p.Stime
			childPid := p.Pid
			code := p.ExitCode
			Release(p)
			if statAddr != nil {
				*statAddr = code
			}
			return childPid

		default:
			flag = 1
			continue
		}
	}

	// exit.c lines 183-193
	if flag != 0 {
		if options&WNOHANG != 0 {
			return 0
		}
		Current.State = TASK_INTERRUPTIBLE
		Schedule()
		// exit.c line 188: check if woken by SIGCHLD
		if Current.Signal&^(1<<(SIGCHLD-1)) == 0 {
			Current.Signal &= ^int32(1 << (SIGCHLD - 1))
			goto repeat
		}
		return -EINTR
	}
	return -ECHILD
}
