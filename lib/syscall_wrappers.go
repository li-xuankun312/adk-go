// lib/syscall_wrappers.go — ported from linux-0.11/lib/_exit.c, close.c,
// dup.c, errno.c, execve.c, open.c, setsid.c, wait.c, write.c
// (C) 1991 Linus Torvalds
//
// These are userspace library wrappers around syscalls.
// In the original C, they use inline assembly to invoke int 0x80.
// In Go, they call the kernel functions directly via function variables.
package lib

// Errno: global error number (per-task in a real OS, but global here)
var Errno int

// Function variables to call kernel syscalls
var (
	SysExitFn   func(int)
	SysCloseFn  func(int) int
	SysDupFn    func(uint32) int
	SysOpenFn   func(string, int, int) int
	SysWriteFn  func(int, []byte, int) int
	SysSetsidFn func() int
	SysWaitpidFn func(int, *int, int) int
	SysExecveFn func(string, []string, []string) int
)

// lib/_exit.c: _exit
func Exit(exitCode int) {
	if SysExitFn != nil { SysExitFn(exitCode) }
}

// lib/close.c: close
func Close(fd int) int {
	if SysCloseFn != nil { return SysCloseFn(fd) }
	return -1
}

// lib/dup.c: dup
func Dup(fd uint32) int {
	if SysDupFn != nil { return SysDupFn(fd) }
	return -1
}

// lib/open.c: open
func Open(filename string, flag, mode int) int {
	if SysOpenFn != nil { return SysOpenFn(filename, flag, mode) }
	return -1
}

// lib/write.c: write
func Write(fd int, buf []byte, count int) int {
	if SysWriteFn != nil { return SysWriteFn(fd, buf, count) }
	return -1
}

// lib/setsid.c: setsid
func Setsid() int {
	if SysSetsidFn != nil { return SysSetsidFn() }
	return -1
}

// lib/wait.c: waitpid
func Waitpid(pid int, stat *int, options int) int {
	if SysWaitpidFn != nil { return SysWaitpidFn(pid, stat, options) }
	return -1
}

// lib/execve.c: execve
func Execve(file string, argv, envp []string) int {
	if SysExecveFn != nil { return SysExecveFn(file, argv, envp) }
	return -1
}
