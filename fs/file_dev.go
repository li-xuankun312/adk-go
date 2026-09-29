// fs/file_dev.go — ported from linux-0.11/fs/file_dev.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

// file_dev.c lines 194-223: file_read
func FileRead(inode *MInode, filp *File, buf []byte, count int) int {
	left := count
	if left <= 0 { return 0 }
	bufIdx := 0
	for left > 0 {
		nr := Bmap(inode, int(filp.FPos)/BLOCK_SIZE)
		var bh *BufferHead
		if nr != 0 {
			bh = Bread(int(inode.IDev), nr)
			if bh == nil { break }
		}
		off := int(filp.FPos) % BLOCK_SIZE
		chars := BLOCK_SIZE - off
		if chars > left { chars = left }
		filp.FPos += int64(chars)
		left -= chars
		if bh != nil {
			for i := 0; i < chars; i++ {
				if bufIdx < len(buf) && off+i < len(bh.BData) {
					buf[bufIdx] = bh.BData[off+i]
				}
				bufIdx++
			}
			Brelse(bh)
		} else {
			for i := 0; i < chars; i++ {
				if bufIdx < len(buf) { buf[bufIdx] = 0 }
				bufIdx++
			}
		}
	}
	inode.IAtime = uint32(CURRENT_TIME())
	if count-left > 0 { return count - left }
	return -1
}

// file_dev.c lines 225-267: file_write
func FileWrite(inode *MInode, filp *File, buf []byte, count int) int {
	var pos int64
	if filp.FFlags&uint16(O_APPEND) != 0 {
		pos = int64(inode.ISize)
	} else {
		pos = filp.FPos
	}
	i := 0
	bufIdx := 0
	for i < count {
		block := CreateBlock(inode, int(pos)/BLOCK_SIZE)
		if block == 0 { break }
		bh := Bread(int(inode.IDev), block)
		if bh == nil { break }
		c := int(pos) % BLOCK_SIZE
		p := c
		bh.BDirt = 1
		c = BLOCK_SIZE - c
		if c > count-i { c = count - i }
		pos += int64(c)
		if uint32(pos) > inode.ISize {
			inode.ISize = uint32(pos)
			inode.IDirt = 1
		}
		i += c
		for j := 0; j < c; j++ {
			if p+j < len(bh.BData) && bufIdx < len(buf) {
				bh.BData[p+j] = buf[bufIdx]
			}
			bufIdx++
		}
		Brelse(bh)
	}
	ct := uint32(CURRENT_TIME())
	inode.IMtime = ct
	if filp.FFlags&uint16(O_APPEND) == 0 {
		filp.FPos = pos
		inode.ICtime = ct
	}
	if i != 0 { return i }
	return -1
}
