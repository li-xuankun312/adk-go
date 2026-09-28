package kernel
import (
	"os"
	"time"
)
func sys_ftime() int64                 { return time.Now().Unix() }
func sys_break() int                   { return -1 }
func sys_ptrace() int                  { return -1 }
func sys_stty() int                    { return -1 }
func sys_gtty() int                    { return -1 }
func sys_rename(old, new string) error { return os.Rename(old, new) }
func sys_prof() int                    { return -1 }
func sys_setregid(rgid, egid int) int  { return 0 }
func sys_setgid(gid int) int           { return 0 }
func sys_acct() int                    { return -1 }
func sys_phys() int                    { return -1 }
func sys_lock() int                    { return -1 }
func sys_mpx() int                     { return -1 }
func sys_ulimit() int                  { return -1 }
func sys_time() int64                  { return time.Now().Unix() }
func sys_setreuid(ruid, euid int) int  { return 0 }
func sys_setuid(uid int) int            { return 0 }
func sys_stime(tp int64) int           { return 0 }
func sys_times() (int64, int64, int64, int64) {
	if current == nil { return 0, 0, 0, 0 }
	return current.utime, current.stime, current.cutime, current.cstime
}
func sys_brk(end_data_seg int64) int64 { return end_data_seg }
func sys_setpgid(pid, pgid int64) int {
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < NR_TASKS; i++ {
		if task[i] != nil && task[i].pid == pid {
			task[i].pgrp = pgid
			return 0
		}
	}
	return -1
}
func sys_getpgrp() int64 {
	if current == nil { return 0 }
	return current.pgrp
}
func sys_setsid() int64 {
	if current == nil { return -1 }
	current.leader = 1
	current.session = current.pid
	current.pgrp = current.pid
	current.tty = -1
	return current.pid
}
func sys_uname() (string, string, string) {
	return "Linux", "0.11-go", "adk-go"
}
func sys_umask(mask int) int {
	if current == nil { return 0 }
	old := current.umask
	current.umask = mask
	return old
}
func SysFtime() int64                          { return sys_ftime() }
func SysTime() int64                           { return sys_time() }
func SysTimes() (int64, int64, int64, int64)   { return sys_times() }
func SysBrk(e int64) int64                     { return sys_brk(e) }
func SysSetpgid(pid, pgid int64) int           { return sys_setpgid(pid, pgid) }
func SysGetpgrp() int64                        { return sys_getpgrp() }
func SysSetsid() int64                         { return sys_setsid() }
func SysUname() (string, string, string)       { return sys_uname() }
func SysUmask(mask int) int                    { return sys_umask(mask) }
func SysRename(old, new string) error          { return sys_rename(old, new) }
