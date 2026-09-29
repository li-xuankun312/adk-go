// fs/char_dev.go — ported from linux-0.11/fs/char_dev.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

// Character read/write function type
type crwFn func(rw int, minor uint16, buf []byte, count int, pos *int64) int

// Callbacks for tty
var (
	ttyReadFn  func(minor uint16, buf []byte, count int) int
	ttyWriteFn func(minor uint16, buf []byte, count int) int
)

func SetTtyRead(fn func(uint16, []byte, int) int)  { ttyReadFn = fn }
func SetTtyWrite(fn func(uint16, []byte, int) int)  { ttyWriteFn = fn }

// char_dev.c lines 94-98: rw_ttyx
func rwTtyx(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	if rw == READ && ttyReadFn != nil { return ttyReadFn(minor, buf, count) }
	if rw == WRITE && ttyWriteFn != nil { return ttyWriteFn(minor, buf, count) }
	return -EIO
}

// char_dev.c lines 100-105: rw_tty
func rwTty(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	if Current.Tty < 0 { return -EPERM }
	return rwTtyx(rw, uint16(Current.Tty), buf, count, pos)
}

// char_dev.c lines 107-154: rw_memory — /dev/mem, /dev/null, etc
func rwMemory(rw int, minor uint16, buf []byte, count int, pos *int64) int {
	switch minor {
	case 3: // /dev/null
		if rw == READ { return 0 }
		return count
	default:
		return -EIO
	}
}

// char_dev.c lines 158-166: crw_table
const NRDEVS_CRW = 8

var crwTable [NRDEVS_CRW]crwFn

func init() {
	crwTable[1] = rwMemory   // /dev/mem etc
	crwTable[4] = rwTtyx     // /dev/ttyx
	crwTable[5] = rwTty      // /dev/tty
}

// char_dev.c lines 168-177: rw_char
func RwChar(rw int, dev uint16, buf []byte, count int, pos *int64) int {
	major := int(MAJOR(uint32(dev)))
	if major >= NRDEVS_CRW { return -ENODEV }
	if crwTable[major] == nil { return -ENODEV }
	return crwTable[major](rw, uint16(MINOR(uint32(dev))), buf, count, pos)
}
