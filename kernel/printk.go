// kernel/printk.go — ported from linux-0.11/kernel/printk.c, panic.c,
// vsprintf.c, mktime.c, traps.c
// (C) 1991 Linus Torvalds
package kernel

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

// ============================================================
// printk.c — kernel printf
// ============================================================

// printk.c: the actual printk implementation
// Original uses vsprintf to format into a buffer, then calls tty_write.
// In Go, we just write to stderr.
func Printk(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

// ============================================================
// panic.c — kernel panic
// ============================================================

// panic.c: void panic(const char * s)
func Panic(s string) {
	Printk("Kernel panic: %s\n", s)
	// In the original, this loops forever with sti().
	// In Go, we exit or use runtime.Goexit().
	runtime.Goexit()
}

// ============================================================
// vsprintf.c — formatted string output
// ============================================================

// vsprintf.c: int vsprintf(char *buf, const char *fmt, va_list args)
// The original is 235 lines implementing printf formatting from scratch.
// In Go, fmt.Sprintf does this.
func Vsprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

func Sprintf(format string, args ...interface{}) string {
	return Vsprintf(format, args...)
}

// ============================================================
// mktime.c — kernel_mktime
// ============================================================

// mktime.c: long kernel_mktime(struct tm * tm)
// Converts broken-down time to Unix timestamp.
func KernelMktime(year, mon, day, hour, min, sec int) int32 {
	t := time.Date(year, time.Month(mon), day, hour, min, sec, 0, time.UTC)
	return int32(t.Unix())
}

// ============================================================
// traps.c — exception handlers
// ============================================================

// traps.c lines 1-208: exception/trap handlers
// Original sets up IDT entries for CPU exceptions (divide error, debug,
// NMI, breakpoint, overflow, etc.) and provides die() for printing
// register state.
//
// In Go, these CPU exceptions don't exist. We provide the die() function
// and stub handlers for completeness.

func die(str string) {
	log.Printf("kernel: %s", str)
	fmt.Fprintf(os.Stderr, "kernel trap: %s\n", str)
	// Original prints register state from stack frame.
	// In Go, we just print a stack trace.
	runtime.Goexit()
}

func DoDivideError()              { die("divide error") }
func DoDebug()                    { die("debug") }
func DoNmi()                      { die("nmi") }
func DoInt3()                     { die("int3 breakpoint") }
func DoOverflow()                 { die("overflow") }
func DoBounds()                   { die("bounds") }
func DoInvalidOp()                { die("invalid operand") }
func DoDeviceNotAvailable()       { die("device not available") }
func DoDoubleFault()              { die("double fault") }
func DoCoprocessorSegmentOverrun() { die("coprocessor segment overrun") }
func DoInvalidTSS()               { die("invalid TSS") }
func DoSegmentNotPresent()        { die("segment not present") }
func DoStackSegment()             { die("stack segment") }
func DoGeneralProtection()        { die("general protection") }
func DoPageFault()                { die("page fault") }
func DoCoprocessorError()         { die("coprocessor error") }

// trap_init: sets up IDT entries — no-op in Go
func TrapInit() {
	log.Printf("kernel: trap_init done")
}
