// fs/ioctl.go — ported from linux-0.11/fs/ioctl.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

// ioctl.c: ioctl dispatch table — function pointers for each major device
var ioctlTable [8]func(dev, cmd, arg int) int

const NRDEVS_IOCTL = 8

// RegisterIoctl allows chr_drv to register ioctl handlers
func RegisterIoctl(major int, fn func(dev, cmd, arg int) int) {
	if major >= 0 && major < NRDEVS_IOCTL { ioctlTable[major] = fn }
}

// ioctl.c lines 272-288: sys_ioctl
func SysIoctl(fd, cmd uint32, arg int) int {
	if fd >= NR_OPEN || Current.Filp[fd] == nil { return -EBADF }
	filp := Current.Filp[fd]
	mode := filp.FInode.IMode
	if !S_ISCHR(mode) && !S_ISBLK(mode) { return -EINVAL_FS }
	dev := int(filp.FInode.IZone[0])
	major := int(MAJOR(uint32(dev)))
	if major >= NRDEVS_IOCTL { return -ENODEV }
	if ioctlTable[major] == nil { return -ENOTTY }
	return ioctlTable[major](dev, int(cmd), arg)
}
