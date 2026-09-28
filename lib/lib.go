package lib
import (
	"fmt"
	"os"
	"syscall"
)
func _exit(code int) { os.Exit(code) }
func close(fd int) error { return syscall.Close(fd) }
func dup(fd int) (int, error) {
	newfd, err := syscall.Dup(fd)
	return newfd, err
}
func execve(path string, argv []string, envp []string) error {
	return syscall.Exec(path, argv, envp)
}
func open(path string, flags int) (int, error) {
	return syscall.Open(path, flags, 0)
}
func write(fd int, buf []byte) (int, error) {
	return syscall.Write(fd, buf)
}
func read(fd int, buf []byte) (int, error) {
	return syscall.Read(fd, buf)
}
func setsid() (int, error) {
	pid, err := syscall.Setsid()
	return pid, err
}
func waitpid(pid int, options int) (int, error) {
	var status syscall.WaitStatus
	wpid, err := syscall.Wait4(pid, &status, options, nil)
	return wpid, err
}
func printk(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
}
func panic(msg string) {
	fmt.Fprintf(os.Stderr, "Kernel panic: %s\n", msg)
	os.Exit(1)
}
func Printk(format string, args ...interface{}) { printk(format, args...) }
func Panic(msg string)                          { panic(msg) }
func Exit(code int)                             { _exit(code) }
func Setsid() (int, error)                      { return setsid() }
