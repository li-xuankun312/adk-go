// fs/ioctl.go — ported from linux-0.11/fs/ioctl.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

const ENOTTY = 25

type ioctlPtr func(dev int, cmd int, arg int) int

// Callback for tty_ioctl
var ttyIoctlFn func(int, int, int) int

func SetTtyIoctl(fn func(int, int, int) int) { ttyIoctlFn = fn }

// ioctl.c lines 150-158: ioctl_table
var ioctlTable = [...]ioctlPtr{
	nil,  // 0: nodev
	nil,  // 1: /dev/mem
	nil,  // 2: /dev/fd
	nil,  // 3: /dev/hd
	nil,  // 4: /dev/ttyx — filled at init
	nil,  // 5: /dev/tty  — filled at init
	nil,  // 6: /dev/lp
	nil,  // 7: named pipes
}

func init() {
	// Wire tty_ioctl for majors 4 and 5
	wrapper := func(dev, cmd, arg int) int {
		if ttyIoctlFn != nil { return ttyIoctlFn(dev, cmd, arg) }
		return -ENOTTY
	}
	ioctlTable[4] = wrapper
	ioctlTable[5] = wrapper
}

// ioctl.c lines 161-177: sys_ioctl
func SysIoctl(fd uint32, cmd uint32, arg uint32) int {
	if fd >= NR_OPEN || Current.Filp[fd] == nil { return -EBADF }
	filp := Current.Filp[fd]
	if filp.FInode == nil { return -EBADF }
	mode := filp.FInode.IMode
	if !S_ISCHR(mode) && !S_ISBLK(mode) { return -EINVAL_FS }
	dev := int(filp.FInode.IZone[0])
	major := int(MAJOR(uint32(dev)))
	if major >= len(ioctlTable) { return -ENODEV }
	if ioctlTable[major] == nil { return -ENOTTY }
	return ioctlTable[major](dev, int(cmd), int(arg))
}
