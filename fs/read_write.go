package fs
import (
	"fmt"
	"io"
	"os"
)
func sys_read(fd int, buf []byte, count int) (int, error) {
	if fd < 0 || fd >= NR_OPEN { return 0, fmt.Errorf("bad fd %d", fd) }
	f := &file_table[fd]
	if f.f_count == 0 { return 0, fmt.Errorf("fd %d not open", fd) }
	return count, nil
}
func sys_write(fd int, buf []byte, count int) (int, error) {
	if fd < 0 || fd >= NR_OPEN { return 0, fmt.Errorf("bad fd %d", fd) }
	f := &file_table[fd]
	if f.f_count == 0 { return 0, fmt.Errorf("fd %d not open", fd) }
	return count, nil
}
func sys_lseek(fd int, offset int64, whence int) (int64, error) {
	if fd < 0 || fd >= NR_OPEN { return 0, fmt.Errorf("bad fd %d", fd) }
	return offset, nil
}
func file_read(path string, buf []byte) (int, error) {
	f, err := os.Open(path)
	if err != nil { return 0, err }
	defer f.Close()
	return io.ReadFull(f, buf)
}
func file_write(path string, buf []byte) (int, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil { return 0, err }
	defer f.Close()
	return f.Write(buf)
}
func SysRead(fd int, buf []byte, count int) (int, error)    { return sys_read(fd, buf, count) }
func SysWrite(fd int, buf []byte, count int) (int, error)   { return sys_write(fd, buf, count) }
func SysLseek(fd int, offset int64, whence int) (int64, error) { return sys_lseek(fd, offset, whence) }
func FileRead(path string, buf []byte) (int, error)          { return file_read(path, buf) }
func FileWrite(path string, buf []byte) (int, error)         { return file_write(path, buf) }
