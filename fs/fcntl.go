// fs/fcntl.go — ported from linux-0.11/fs/fcntl.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

// fcntl constants
const (
	F_DUPFD  = 0
	F_GETFD  = 1
	F_SETFD  = 2
	F_GETFL  = 3
	F_SETFL  = 4
	F_GETLK  = 5
	F_SETLK  = 6
	F_SETLKW = 7
	O_APPEND   = 02000
	O_NONBLOCK = 04000
	EMFILE = 24
	ENODEV = 19
	ENOTTY = 25
)

// fcntl.c lines 185-201: dupfd
func dupfd(fd, arg uint32) int {
	if fd >= NR_OPEN || Current.Filp[fd] == nil { return -EBADF }
	if arg >= NR_OPEN { return -EINVAL_FS }
	for arg < NR_OPEN {
		if Current.Filp[arg] == nil { break }
		arg++
	}
	if arg >= NR_OPEN { return -EMFILE }
	Current.CloseOnExec &^= uint32(1 << arg)
	Current.Filp[arg] = Current.Filp[fd]
	Current.Filp[arg].FCount++
	return int(arg)
}

// fcntl.c lines 203-207: sys_dup2
func SysDup2(oldfd, newfd uint32) int {
	SysCloseFS(int(newfd))
	return dupfd(oldfd, newfd)
}

// fcntl.c lines 209-212: sys_dup
func SysDup(fildes uint32) int {
	return dupfd(fildes, 0)
}

// fcntl.c lines 214-242: sys_fcntl
func SysFcntl(fd, cmd, arg uint32) int {
	if fd >= NR_OPEN || Current.Filp[fd] == nil { return -EBADF }
	filp := Current.Filp[fd]
	switch cmd {
	case F_DUPFD:
		return dupfd(fd, arg)
	case F_GETFD:
		return int((Current.CloseOnExec >> fd) & 1)
	case F_SETFD:
		if arg&1 != 0 {
			Current.CloseOnExec |= 1 << fd
		} else {
			Current.CloseOnExec &^= 1 << fd
		}
		return 0
	case F_GETFL:
		return int(filp.FFlags)
	case F_SETFL:
		filp.FFlags &^= uint16(O_APPEND | O_NONBLOCK)
		filp.FFlags |= uint16(arg) & uint16(O_APPEND|O_NONBLOCK)
		return 0
	case F_GETLK, F_SETLK, F_SETLKW:
		return -1
	default:
		return -1
	}
}
