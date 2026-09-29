// fs/char_dev.go — ported from linux-0.11/fs/char_dev.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

const ENODEV = 19

// char_dev.c: crw_ptr = func(rw int, minor uint16, buf []byte, count int, pos *int64) int
type crwPtr func(rw int, minor uint16, buf []byte, count int, pos *int64) int

// Callbacks for tty read/write
var (
	ttyReadFn  func(minor uint16, buf []byte, count int) int
	ttyWriteFn func(minor uint16, buf []byte, count int) int
)

func SetTtyRead(fn func(uint16, []byte, int) int)  { ttyReadFn = fn }
func SetTtyWrite(fn func(uint16, []byte, int) int) { ttyWriteFn = fn }

// char_dev.c lines 21-25: rw_ttyx
func rwTtyx(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	if rw == READ {
		if ttyReadFn != nil { return ttyReadFn(minor, buf, count) }
		return -EIO
	}
	if ttyWriteFn != nil { return ttyWriteFn(minor, buf, count) }
	return -EIO
}

// char_dev.c lines 27-32: rw_tty
func rwTty(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	if Current.Tty < 0 { return -EPERM }
	return rwTtyx(rw, uint16(Current.Tty), buf, count, pos)
}

// char_dev.c lines 34-47: rw_ram, rw_mem, rw_kmem — stubs
func rwRam(rw int, minor uint16, buf []byte, count int, pos *int64) int  { return -EIO }
func rwMem(rw int, minor uint16, buf []byte, count int, pos *int64) int  { return -EIO }
func rwKmem(rw int, minor uint16, buf []byte, count int, pos *int64) int { return -EIO }

// char_dev.c lines 49-63: rw_port — stub for Go port
func rwPort(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	// I/O port access not meaningful in Go; stub returns 0
	return 0
}

// char_dev.c lines 65-81: rw_memory
func rwMemory(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	switch minor {
	case 0: return rwRam(rw, minor, buf, count, pos)
	case 1: return rwMem(rw, minor, buf, count, pos)
	case 2: return rwKmem(rw, minor, buf, count, pos)
	case 3: // /dev/null
		if rw == READ { return 0 }
		return count
	case 4: return rwPort(rw, minor, buf, count, pos)
	default: return -EIO
	}
}

// char_dev.c lines 85-93: crw_table
var crwTable = [...]crwPtr{
	nil,        // 0: nodev
	rwMemory,   // 1: /dev/mem etc
	nil,        // 2: /dev/fd
	nil,        // 3: /dev/hd
	rwTtyx,     // 4: /dev/ttyx
	rwTty,      // 5: /dev/tty
	nil,        // 6: /dev/lp
	nil,        // 7: unnamed pipes
}

// char_dev.c lines 95-104: rw_char
func RwChar(rw int, dev uint16, buf []byte, count int, pos *int64) int {
	major := MAJOR(uint32(dev))
	if int(major) >= len(crwTable) { return -ENODEV }
	callAddr := crwTable[major]
	if callAddr == nil { return -ENODEV }
	return callAddr(rw, uint16(MINOR(uint32(dev))), buf, count, pos)
}
