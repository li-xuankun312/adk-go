// kernel/signal.go — ported from linux-0.11/kernel/signal.c
// (C) 1991 Linus Torvalds
package kernel

import (
	. "google.golang.org/adk/v2/include"
)

// signal.c lines 15-18: sys_sgetmask
func SysSgetmask() int32 {
	return Current.Blocked
}

// signal.c lines 20-26: sys_ssetmask
func SysSsetmask(newmask int32) int32 {
	old := Current.Blocked
	Current.Blocked = newmask & ^(1 << (SIGKILL - 1))
	return old
}

// signal.c lines 28-46: save_old and get_new
// In C, these copy sigaction structs between user space and kernel space
// using put_fs_byte/get_fs_byte. In Go, we just copy the struct directly.

// signal.c lines 48-61: sys_signal
// Original: int sys_signal(int signum, long handler, long restorer)
func SysSignal(signum int32, handler func(int), restorer func()) int32 {
	if signum < 1 || signum > 32 || signum == SIGKILL {
		return -1
	}
	// Build new sigaction
	handlerType := SIG_HANDLER_CUSTOM
	if handler == nil {
		handlerType = SIG_HANDLER_DEFAULT
	}
	tmp := Sigaction{
		SaHandler:     handler,
		SaHandlerType: handlerType,
		SaMask:        0,
		SaFlags:       SA_ONESHOT | SA_NOMASK,
		SaRestorer:    restorer,
	}
	// Save old handler (return value represents old handler in C)
	// In Go, we can't return a function as int32, so we just return 0.
	// The important thing is that the handler is installed.
	Current.Sigaction[signum-1] = tmp
	return 0
}

// signal.c lines 63-80: sys_sigaction
func SysSigaction(signum int32, action *Sigaction, oldaction *Sigaction) int32 {
	if signum < 1 || signum > 32 || signum == SIGKILL {
		return -1
	}

	// Save old
	tmp := Current.Sigaction[signum-1]

	// Install new
	if action != nil {
		Current.Sigaction[signum-1] = *action
	}

	// Return old
	if oldaction != nil {
		*oldaction = tmp
	}

	// signal.c lines 75-78: set auto-mask
	if Current.Sigaction[signum-1].SaFlags&SA_NOMASK != 0 {
		Current.Sigaction[signum-1].SaMask = 0
	} else {
		Current.Sigaction[signum-1].SaMask |= uint32(1 << (signum - 1))
	}
	return 0
}

// signal.c lines 82-119: do_signal
// Original manipulates the user-mode stack to set up signal handler return.
// In Go, we can't manipulate a hardware stack, so we invoke the handler
// directly if one is installed.
//
// This is called from the signal-check path (after system calls return).
func DoSignal(signr int32) {
	if signr < 1 || signr > 32 {
		return
	}
	sa := &Current.Sigaction[signr-1]

	// signal.c line 94-95: SIG_IGN (sa_handler==1)
	if sa.SaHandlerType == SIG_HANDLER_IGNORE {
		return
	}

	// signal.c lines 96-101: SIG_DFL (sa_handler==0)
	if sa.SaHandlerType == SIG_HANDLER_DEFAULT {
		if signr == SIGCHLD {
			return
		}
		DoExit(1 << (signr - 1))
		return
	}

	// signal.c lines 102-103: SA_ONESHOT
	if sa.SaFlags&SA_ONESHOT != 0 {
		sa.SaHandler = nil
		sa.SaHandlerType = SIG_HANDLER_DEFAULT
	}

	// signal.c lines 104-118: call the handler
	// In C, this manipulates the stack frame. In Go, we just call it.
	if sa.SaHandler != nil {
		sa.SaHandler(int(signr))
	}

	// signal.c line 118: mask signal during handler
	Current.Blocked |= int32(sa.SaMask)
}
