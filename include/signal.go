// include/signal.go — ported from linux-0.11/include/signal.h
// Line-for-line translation. See original for Linus's comments.
package include

// signal.h lines 39-64: signal numbers
const (
	NSIG    = 32
	SIGHUP  = 1
	SIGINT  = 2
	SIGQUIT = 3
	SIGILL  = 4
	SIGTRAP = 5
	SIGABRT = 6
	SIGIOT  = 6
	SIGUNUSED = 7
	SIGFPE  = 8
	SIGKILL = 9
	SIGUSR1 = 10
	SIGSEGV = 11
	SIGUSR2 = 12
	SIGPIPE = 13
	SIGALRM = 14
	SIGTERM = 15
	SIGSTKFLT = 16
	SIGCHLD = 17
	SIGCONT = 18
	SIGSTOP = 19
	SIGTSTP = 20
	SIGTTIN = 21
	SIGTTOU = 22
)

// signal.h lines 67-69: sigaction flags
const (
	SA_NOCLDSTOP = 1
	SA_NOMASK    = 0x40000000
	SA_ONESHOT   = 0x80000000
)

// signal.h lines 71-73: sigprocmask how values
const (
	SIG_BLOCK   = 0
	SIG_UNBLOCK = 1
	SIG_SETMASK = 2
)

// signal.h line 78-83: struct sigaction
// Original:
//   struct sigaction {
//       void (*sa_handler)(int);
//       sigset_t sa_mask;
//       int sa_flags;
//       void (*sa_restorer)(void);
//   };
//
// sa_restorer is x86 signal-return trampoline; we keep the field but it's unused.
type Sigaction struct {
	SaHandler     func(int) // void (*sa_handler)(int) — nil when SIG_DFL or SIG_IGN
	SaHandlerType int       // 0=SIG_DFL, 1=SIG_IGN, 2=custom handler
	SaMask        uint32    // sigset_t sa_mask (32 bits)
	SaFlags       int32     // int sa_flags
	SaRestorer    func()    // void (*sa_restorer)(void) — unused in Go port
}

// SIG_DFL and SIG_IGN: in C these are (void (*)(int))0 and (void (*)(int))1.
// In Go, functions aren't comparable, so we use a separate integer field
// in Sigaction to represent SIG_DFL (0) and SIG_IGN (1).
// When SaHandlerType is SIG_HANDLER_DEFAULT or SIG_HANDLER_IGNORE,
// SaHandler is nil and should not be called.
const (
	SIG_HANDLER_DEFAULT = 0 // SIG_DFL
	SIG_HANDLER_IGNORE  = 1 // SIG_IGN
	SIG_HANDLER_CUSTOM  = 2 // user handler installed
)
