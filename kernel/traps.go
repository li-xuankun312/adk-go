package kernel
import (
	"fmt"
	"log"
	"os"
	"runtime"
)
func die(str string) {
	log.Printf("kernel: %s", str)
	fmt.Fprintf(os.Stderr, "kernel panic: %s\n", str)
	runtime.Goexit()
}
func do_divide_error()           { die("divide error") }
func do_debug()                  { die("debug") }
func do_nmi()                    { die("nmi") }
func do_int3()                   { die("int3 breakpoint") }
func do_overflow()               { die("overflow") }
func do_bounds()                 { die("bounds") }
func do_invalid_op()             { die("invalid operand") }
func do_device_not_available()   { die("device not available") }
func do_double_fault()           { die("double fault") }
func do_coprocessor_segment_overrun() { die("coprocessor segment overrun") }
func do_invalid_TSS()            { die("invalid TSS") }
func do_segment_not_present()    { die("segment not present") }
func do_stack_segment()          { die("stack segment") }
func do_general_protection()     { die("general protection") }
func do_page_fault()             { die("page fault") }
func do_coprocessor_error()      { die("coprocessor error") }
func trap_init() {
	log.Printf("kernel: trap_init done")
}
