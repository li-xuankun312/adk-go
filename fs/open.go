package fs
import (
	"fmt"
	"os"
	"syscall"
)
func sys_open(filename string, flag int, mode int) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	fd := -1
	for i := 0; i < NR_FILE; i++ {
		if file_table[i].f_count == 0 { fd = i; break }
	}
	if fd < 0 { return -1, fmt.Errorf("sys_open: no free file slots") }
	f := &file_table[fd]
	f.f_mode = uint16(flag & 3)
	f.f_flags = uint16(flag)
	f.f_count = 1
	f.f_pos = 0
	return fd, nil
}
func sys_close(fd int) error {
	if fd < 0 || fd >= NR_FILE { return fmt.Errorf("bad fd") }
	mu.Lock()
	defer mu.Unlock()
	if file_table[fd].f_count == 0 { return fmt.Errorf("fd not open") }
	file_table[fd].f_count--
	return nil
}
func sys_creat(pathname string, mode int) (int, error) {
	return sys_open(pathname, syscall.O_CREAT|syscall.O_TRUNC|syscall.O_WRONLY, mode)
}
func sys_link(oldname, newname string) error {
	return os.Link(oldname, newname)
}
func sys_unlink(name string) error {
	return os.Remove(name)
}
func sys_mkdir(pathname string, mode int) error {
	return os.Mkdir(pathname, os.FileMode(mode))
}
func sys_rmdir(pathname string) error {
	return os.Remove(pathname)
}
func sys_chdir(pathname string) error {
	return os.Chdir(pathname)
}
func sys_chmod(filename string, mode int) error {
	return os.Chmod(filename, os.FileMode(mode))
}
func sys_chown(filename string, uid, gid int) error {
	return os.Chown(filename, uid, gid)
}
func sys_stat(filename string) (os.FileInfo, error) {
	return os.Stat(filename)
}
func sys_fstat(fd int) (os.FileInfo, error) {
	return nil, fmt.Errorf("fstat: not implemented for virtual fd")
}
func SysOpen(f string, flag, mode int) (int, error) { return sys_open(f, flag, mode) }
func SysClose(fd int) error                         { return sys_close(fd) }
func SysCreat(p string, m int) (int, error)          { return sys_creat(p, m) }
func SysLink(o, n string) error                      { return sys_link(o, n) }
func SysUnlink(n string) error                       { return sys_unlink(n) }
func SysMkdir(p string, m int) error                 { return sys_mkdir(p, m) }
func SysRmdir(p string) error                        { return sys_rmdir(p) }
func SysChdir(p string) error                        { return sys_chdir(p) }
func SysChmod(f string, m int) error                 { return sys_chmod(f, m) }
func SysChown(f string, u, g int) error              { return sys_chown(f, u, g) }
func SysStat(f string) (os.FileInfo, error)          { return sys_stat(f) }
