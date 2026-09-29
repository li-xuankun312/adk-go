// fs/inode.go — ported from linux-0.11/fs/inode.c
// (C) 1991 Linus Torvalds
package fs

import (
	"encoding/binary"
	"log"
	"sync"

	. "google.golang.org/adk/v2/include"
)

// inode.c line 15: struct m_inode inode_table[NR_INODE]
var InodeTable [NR_INODE]MInode

// inode.c line 197: static struct m_inode * last_inode = inode_table
var lastInodeIdx int

var inodeMu sync.Mutex // replaces cli/sti

// Callbacks for cross-package calls
var (
	getFrePageFn func() uint32
	freePageFn   func(uint32)
)

func SetGetFreePage(fn func() uint32) { getFrePageFn = fn }
func SetFreePage(fn func(uint32))     { freePageFn = fn }

// inode.c lines 20-26: wait_on_inode
func waitOnInode(inode *MInode) {
	inodeMu.Lock()
	for inode.ILock != 0 {
		inodeMu.Unlock()
		if sleepOnFn != nil {
			sleepOnFn(&inode.IWait)
		}
		inodeMu.Lock()
	}
	inodeMu.Unlock()
}

// inode.c lines 28-35: lock_inode
func lockInode(inode *MInode) {
	inodeMu.Lock()
	for inode.ILock != 0 {
		inodeMu.Unlock()
		if sleepOnFn != nil {
			sleepOnFn(&inode.IWait)
		}
		inodeMu.Lock()
	}
	inode.ILock = 1
	inodeMu.Unlock()
}

// inode.c lines 37-41: unlock_inode
func unlockInode(inode *MInode) {
	inode.ILock = 0
	if wakeUpFn != nil {
		wakeUpFn(&inode.IWait)
	}
}

// inode.c lines 43-57: invalidate_inodes
func InvalidateInodes(dev int) {
	for i := 0; i < NR_INODE; i++ {
		inode := &InodeTable[i]
		waitOnInode(inode)
		if inode.IDev == uint16(dev) {
			if inode.ICount != 0 {
				log.Printf("fs: inode in use on removed disk")
			}
			inode.IDev = 0
			inode.IDirt = 0
		}
	}
}

// inode.c lines 59-70: sync_inodes
func SyncInodes() {
	if syncInodesFn != nil {
		// Use override if set
		syncInodesFn()
		return
	}
	for i := 0; i < NR_INODE; i++ {
		inode := &InodeTable[i]
		waitOnInode(inode)
		if inode.IDirt != 0 && inode.IPipe == 0 {
			writeInode(inode)
		}
	}
}

// INODES_PER_BLOCK: sizeof(d_inode) in Linux 0.11 is 32 bytes
// BLOCK_SIZE / 32 = 32
const INODES_PER_BLOCK = BLOCK_SIZE / 32

// inode.c lines 72-138: _bmap
// Maps logical block number to physical block number.
// If create is true, allocates new blocks as needed.
func bmap_internal(inode *MInode, block int, create int) int {
	if block < 0 {
		log.Printf("fs: _bmap: block<0")
		return 0
	}
	if block >= 7+512+512*512 {
		log.Printf("fs: _bmap: block>big")
		return 0
	}
	// Direct blocks (0-6)
	if block < 7 {
		if create != 0 && inode.IZone[block] == 0 {
			nb := NewBlock(int(inode.IDev))
			if nb != 0 {
				inode.IZone[block] = uint16(nb)
				inode.ICtime = uint32(CURRENT_TIME())
				inode.IDirt = 1
			}
		}
		return int(inode.IZone[block])
	}
	// Indirect block (7-518)
	block -= 7
	if block < 512 {
		if create != 0 && inode.IZone[7] == 0 {
			nb := NewBlock(int(inode.IDev))
			if nb != 0 {
				inode.IZone[7] = uint16(nb)
				inode.IDirt = 1
				inode.ICtime = uint32(CURRENT_TIME())
			}
		}
		if inode.IZone[7] == 0 { return 0 }
		bh := Bread(int(inode.IDev), int(inode.IZone[7]))
		if bh == nil { return 0 }
		i := int(binary.LittleEndian.Uint16(bh.BData[block*2 : block*2+2]))
		if create != 0 && i == 0 {
			nb := NewBlock(int(inode.IDev))
			if nb != 0 {
				i = nb
				binary.LittleEndian.PutUint16(bh.BData[block*2:block*2+2], uint16(i))
				bh.BDirt = 1
			}
		}
		Brelse(bh)
		return i
	}
	// Double indirect block (519+)
	block -= 512
	if create != 0 && inode.IZone[8] == 0 {
		nb := NewBlock(int(inode.IDev))
		if nb != 0 {
			inode.IZone[8] = uint16(nb)
			inode.IDirt = 1
			inode.ICtime = uint32(CURRENT_TIME())
		}
	}
	if inode.IZone[8] == 0 { return 0 }
	bh := Bread(int(inode.IDev), int(inode.IZone[8]))
	if bh == nil { return 0 }
	i := int(binary.LittleEndian.Uint16(bh.BData[(block>>9)*2 : (block>>9)*2+2]))
	if create != 0 && i == 0 {
		nb := NewBlock(int(inode.IDev))
		if nb != 0 {
			i = nb
			binary.LittleEndian.PutUint16(bh.BData[(block>>9)*2:(block>>9)*2+2], uint16(i))
			bh.BDirt = 1
		}
	}
	Brelse(bh)
	if i == 0 { return 0 }
	bh = Bread(int(inode.IDev), i)
	if bh == nil { return 0 }
	i = int(binary.LittleEndian.Uint16(bh.BData[(block&511)*2 : (block&511)*2+2]))
	if create != 0 && i == 0 {
		nb := NewBlock(int(inode.IDev))
		if nb != 0 {
			i = nb
			binary.LittleEndian.PutUint16(bh.BData[(block&511)*2:(block&511)*2+2], uint16(i))
			bh.BDirt = 1
		}
	}
	Brelse(bh)
	return i
}

// inode.c lines 140-143: bmap
func Bmap(inode *MInode, block int) int {
	return bmap_internal(inode, block, 0)
}

// inode.c lines 145-148: create_block
func CreateBlock(inode *MInode, block int) int {
	return bmap_internal(inode, block, 1)
}

// S_ISBLK: check if mode indicates block device
func S_ISBLK(mode uint16) bool { return (mode & 0xF000) == 0x6000 }

// inode.c lines 150-192: iput
func Iput(inode *MInode) {
	if inode == nil { return }
	waitOnInode(inode)
	if inode.ICount == 0 {
		log.Printf("fs: iput: trying to free free inode")
		return
	}
	if inode.IPipe != 0 {
		if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
		inode.ICount--
		if inode.ICount != 0 { return }
		if freePageFn != nil { freePageFn(uint32(inode.ISize)) }
		inode.ICount = 0
		inode.IDirt = 0
		inode.IPipe = 0
		return
	}
	if inode.IDev == 0 {
		inode.ICount--
		return
	}
	if S_ISBLK(inode.IMode) {
		SyncDev(int(inode.IZone[0]))
		waitOnInode(inode)
	}
repeat:
	if inode.ICount > 1 {
		inode.ICount--
		return
	}
	if inode.INlinks == 0 {
		Truncate(inode)
		FreeInode(inode)
		return
	}
	if inode.IDirt != 0 {
		writeInode(inode)
		waitOnInode(inode)
		goto repeat
	}
	inode.ICount--
}

// inode.c lines 194-226: get_empty_inode
func GetEmptyInode() *MInode {
	for {
		var inode *MInode
		for i := 0; i < NR_INODE; i++ {
			lastInodeIdx++
			if lastInodeIdx >= NR_INODE { lastInodeIdx = 0 }
			if InodeTable[lastInodeIdx].ICount == 0 {
				inode = &InodeTable[lastInodeIdx]
				if inode.IDirt == 0 && inode.ILock == 0 {
					break
				}
			}
		}
		if inode == nil {
			log.Printf("fs: No free inodes in mem")
			return nil
		}
		waitOnInode(inode)
		for inode.IDirt != 0 {
			writeInode(inode)
			waitOnInode(inode)
		}
		if inode.ICount != 0 { continue }
		// memset(inode, 0, sizeof(*inode))
		*inode = MInode{}
		inode.ICount = 1
		return inode
	}
}

// inode.c lines 228-242: get_pipe_inode
func GetPipeInode() *MInode {
	inode := GetEmptyInode()
	if inode == nil { return nil }
	page := uint32(0)
	if getFrePageFn != nil { page = getFrePageFn() }
	if page == 0 {
		inode.ICount = 0
		return nil
	}
	inode.ISize = page
	inode.ICount = 2 // sum of readers/writers
	inode.IZone[0] = 0 // PIPE_HEAD
	inode.IZone[1] = 0 // PIPE_TAIL
	inode.IPipe = 1
	return inode
}

// inode.c lines 244-292: iget
func Iget(dev, nr int) *MInode {
	if dev == 0 {
		log.Printf("fs: iget with dev==0")
		return nil
	}
	empty := GetEmptyInode()
	for i := 0; i < NR_INODE; i++ {
		inode := &InodeTable[i]
		if inode.IDev != uint16(dev) || inode.INum != uint16(nr) {
			continue
		}
		waitOnInode(inode)
		if inode.IDev != uint16(dev) || inode.INum != uint16(nr) {
			i = -1 // restart
			continue
		}
		inode.ICount++
		if inode.IMount != 0 {
			for j := 0; j < NR_SUPER; j++ {
				if SuperBlockTable[j].SImount == inode { 
					Iput(inode)
					dev = int(SuperBlockTable[j].SDev)
					nr = ROOT_INO
					i = -1 // restart
					break
				}
			}
			if i == -1 { continue }
			log.Printf("fs: Mounted inode hasn't got sb")
			if empty != nil { Iput(empty) }
			return inode
		}
		if empty != nil { Iput(empty) }
		return inode
	}
	if empty == nil { return nil }
	inode := empty
	inode.IDev = uint16(dev)
	inode.INum = uint16(nr)
	readInode(inode)
	return inode
}

// inode.c lines 294-312: read_inode
func readInode(inode *MInode) {
	lockInode(inode)
	sb := GetSuper(int(inode.IDev))
	if sb == nil {
		log.Printf("fs: trying to read inode without dev")
		unlockInode(inode)
		return
	}
	block := 2 + int(sb.SImapBlocks) + int(sb.SZmapBlocks) +
		(int(inode.INum)-1)/INODES_PER_BLOCK
	bh := Bread(int(inode.IDev), block)
	if bh == nil {
		log.Printf("fs: unable to read i-node block")
		unlockInode(inode)
		return
	}
	// Read d_inode from buffer
	idx := (int(inode.INum) - 1) % INODES_PER_BLOCK
	offset := idx * 32 // sizeof(d_inode) = 32
	if offset+32 <= len(bh.BData) {
		inode.IMode = binary.LittleEndian.Uint16(bh.BData[offset:])
		inode.IUid = binary.LittleEndian.Uint16(bh.BData[offset+2:])
		inode.ISize = binary.LittleEndian.Uint32(bh.BData[offset+4:])
		inode.IMtime = binary.LittleEndian.Uint32(bh.BData[offset+8:])
		inode.IGid = bh.BData[offset+12]
		inode.INlinks = bh.BData[offset+13]
		for j := 0; j < 9; j++ {
			inode.IZone[j] = binary.LittleEndian.Uint16(bh.BData[offset+14+j*2:])
		}
	}
	Brelse(bh)
	unlockInode(inode)
}

// inode.c lines 314-338: write_inode
func writeInode(inode *MInode) {
	lockInode(inode)
	if inode.IDirt == 0 || inode.IDev == 0 {
		unlockInode(inode)
		return
	}
	sb := GetSuper(int(inode.IDev))
	if sb == nil {
		log.Printf("fs: trying to write inode without device")
		unlockInode(inode)
		return
	}
	block := 2 + int(sb.SImapBlocks) + int(sb.SZmapBlocks) +
		(int(inode.INum)-1)/INODES_PER_BLOCK
	bh := Bread(int(inode.IDev), block)
	if bh == nil {
		log.Printf("fs: unable to read i-node block")
		unlockInode(inode)
		return
	}
	idx := (int(inode.INum) - 1) % INODES_PER_BLOCK
	offset := idx * 32
	if offset+32 <= len(bh.BData) {
		binary.LittleEndian.PutUint16(bh.BData[offset:], inode.IMode)
		binary.LittleEndian.PutUint16(bh.BData[offset+2:], inode.IUid)
		binary.LittleEndian.PutUint32(bh.BData[offset+4:], inode.ISize)
		binary.LittleEndian.PutUint32(bh.BData[offset+8:], inode.IMtime)
		bh.BData[offset+12] = inode.IGid
		bh.BData[offset+13] = inode.INlinks
		for j := 0; j < 9; j++ {
			binary.LittleEndian.PutUint16(bh.BData[offset+14+j*2:], inode.IZone[j])
		}
	}
	bh.BDirt = 1
	inode.IDirt = 0
	Brelse(bh)
	unlockInode(inode)
}
