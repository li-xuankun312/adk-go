package kernel

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"time"
)

func Printk(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func Panic(s string) {
	Printk("Kernel panic: %s\n", s)
	runtime.Goexit()
}

func Vsprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

func Sprintf(format string, args ...interface{}) string {
	return Vsprintf(format, args...)
}

func KernelMktime(year, mon, day, hour, min, sec int) int32 {
	t := time.Date(year, time.Month(mon), day, hour, min, sec, 0, time.UTC)
	return int32(t.Unix())
}

func die(str string) {
	log.Printf("kernel: %s", str)
	fmt.Fprintf(os.Stderr, "kernel trap: %s\n", str)
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

func TrapInit() {
	log.Printf("kernel: trap_init done")
}
