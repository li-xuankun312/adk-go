package kernel
import (
	"fmt"
	"os"
)
func printk(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}
func kpanic(msg string) {
	printk("Kernel panic: %s\n", msg)
	os.Exit(1)
}
