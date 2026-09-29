// fs/truncate.go — ported from linux-0.11/fs/truncate.c
// (C) 1991 Linus Torvalds
package fs

import (
	"encoding/binary"

	. "google.golang.org/adk/v2/include"
)

// S_ISREG
func S_ISREG(mode uint16) bool { return (mode & 0xF000) == 0x8000 }

// truncate.c lines 11-27: free_ind
func freeInd(dev, block int) {
	if block == 0 { return }
	bh := Bread(dev, block)
	if bh != nil {
		for i := 0; i < 512; i++ {
			p := int(binary.LittleEndian.Uint16(bh.BData[i*2 : i*2+2]))
			if p != 0 { FreeBlock(dev, p) }
		}
		Brelse(bh)
	}
	FreeBlock(dev, block)
}

// truncate.c lines 29-45: free_dind
func freeDind(dev, block int) {
	if block == 0 { return }
	bh := Bread(dev, block)
	if bh != nil {
		for i := 0; i < 512; i++ {
			p := int(binary.LittleEndian.Uint16(bh.BData[i*2 : i*2+2]))
			if p != 0 { freeInd(dev, p) }
		}
		Brelse(bh)
	}
	FreeBlock(dev, block)
}

// truncate.c lines 47-64: truncate
func Truncate(inode *MInode) {
	if !(S_ISREG(inode.IMode) || S_ISDIR(inode.IMode)) { return }
	for i := 0; i < 7; i++ {
		if inode.IZone[i] != 0 {
			FreeBlock(int(inode.IDev), int(inode.IZone[i]))
			inode.IZone[i] = 0
		}
	}
	freeInd(int(inode.IDev), int(inode.IZone[7]))
	freeDind(int(inode.IDev), int(inode.IZone[8]))
	inode.IZone[7] = 0
	inode.IZone[8] = 0
	inode.ISize = 0
	inode.IDirt = 1
	ct := uint32(CURRENT_TIME())
	inode.IMtime = ct
	inode.ICtime = ct
}
