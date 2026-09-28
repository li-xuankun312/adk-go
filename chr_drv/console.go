package chr_drv
import (
	"fmt"
	"os"
)
const (
	NPAR    = 16
	NR_COLS = 80
	NR_ROWS = 25
)
type console_struct struct {
	x, y      int
	top, bottom int
	attr      byte
	state     int
	par       [NPAR]int
	npar      int
}
var cons console_struct
func con_init() {
	cons = console_struct{
		x: 0, y: 0,
		top: 0, bottom: NR_ROWS,
		attr: 0x07,
	}
}
func con_putc(c byte) {
	switch c {
	case '\n':
		fmt.Fprintln(os.Stdout)
	case '\r':
	case '\t':
		fmt.Fprint(os.Stdout, "\t")
	case '\b':
		fmt.Fprint(os.Stdout, "\b \b")
	default:
		fmt.Fprintf(os.Stdout, "%c", c)
	}
}
func con_puts(s string) {
	for _, c := range s { con_putc(byte(c)) }
}
func ConInit()           { con_init() }
func ConPutc(c byte)     { con_putc(c) }
func ConPuts(s string)   { con_puts(s) }
