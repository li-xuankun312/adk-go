// fs/super.go — ported from linux-0.11/fs/super.c
// (C) 1991 Linus Torvalds
//
// super.c contains code to handle the super-block tables.
package fs

import (
	"encoding/binary"
	"log"
	"sync"

	. "google.golang.org/adk/v2/include"
)

// Error codes
const (
	ENOENT  = 2
	ENOTBLK = 15
	EBUSY   = 16
	EPERM   = 1
)

// super.c line 27
var SuperBlockTable [NR_SUPER]SuperBlock

// super.c line 29: ROOT_DEV is in include/fs.go

var superMu sync.Mutex // replaces cli/sti

// S_ISDIR
func S_ISDIR(mode uint16) bool { return (mode & 0xF000) == 0x4000 }

// super.c lines 31-38: lock_super
func lockSuper(sb *SuperBlock) {
	superMu.Lock()
	for sb.SLock != 0 {
		superMu.Unlock()
		if sleepOnFn != nil { sleepOnFn(&sb.SWait) }
		superMu.Lock()
	}
	sb.SLock = 1
	superMu.Unlock()
}

// super.c lines 40-46: free_super
func freeSuper(sb *SuperBlock) {
	sb.SLock = 0
	if wakeUpFn != nil { wakeUpFn(&sb.SWait) }
}

// super.c lines 48-54: wait_on_super
func waitOnSuper(sb *SuperBlock) {
	superMu.Lock()
	for sb.SLock != 0 {
		superMu.Unlock()
		if sleepOnFn != nil { sleepOnFn(&sb.SWait) }
		superMu.Lock()
	}
	superMu.Unlock()
}

// super.c lines 56-72: get_super
func GetSuper(dev int) *SuperBlock {
	if dev == 0 { return nil }
	for i := 0; i < NR_SUPER; i++ {
		s := &SuperBlockTable[i]
		if s.SDev == uint16(dev) {
			waitOnSuper(s)
			if s.SDev == uint16(dev) { return s }
			i = -1 // restart
			continue
		}
	}
	return nil
}

// super.c lines 74-98: put_super
func PutSuper(dev int) {
	if dev == ROOT_DEV {
		log.Printf("fs: root diskette changed: prepare for armageddon")
		return
	}
	sb := GetSuper(dev)
	if sb == nil { return }
	if sb.SImount != nil {
		log.Printf("fs: Mounted disk changed - tssk, tssk")
		return
	}
	lockSuper(sb)
	sb.SDev = 0
	for i := 0; i < I_MAP_SLOTS; i++ { Brelse(sb.SImap[i]) }
	for i := 0; i < Z_MAP_SLOTS; i++ { Brelse(sb.SZmap[i]) }
	freeSuper(sb)
}

// super.c lines 100-165: read_super
func readSuper(dev int) *SuperBlock {
	if dev == 0 { return nil }
	CheckDiskChange(dev)
	if s := GetSuper(dev); s != nil { return s }
	// Find free slot
	var s *SuperBlock
	for i := 0; i < NR_SUPER; i++ {
		if SuperBlockTable[i].SDev == 0 {
			s = &SuperBlockTable[i]
			break
		}
	}
	if s == nil { return nil }
	s.SDev = uint16(dev)
	s.SIsup = nil
	s.SImount = nil
	s.STime = 0
	s.SRdOnly = 0
	s.SDirt = 0
	lockSuper(s)
	bh := Bread(dev, 1)
	if bh == nil {
		s.SDev = 0; freeSuper(s); return nil
	}
	// Copy d_super_block from buffer
	if len(bh.BData) >= 20 {
		s.SNinodes = binary.LittleEndian.Uint16(bh.BData[0:])
		s.SNzones = binary.LittleEndian.Uint16(bh.BData[2:])
		s.SImapBlocks = binary.LittleEndian.Uint16(bh.BData[4:])
		s.SZmapBlocks = binary.LittleEndian.Uint16(bh.BData[6:])
		s.SFirstdatazone = binary.LittleEndian.Uint16(bh.BData[8:])
		s.SLogZoneSize = binary.LittleEndian.Uint16(bh.BData[10:])
		s.SMaxSize = binary.LittleEndian.Uint32(bh.BData[12:])
		s.SMagic = binary.LittleEndian.Uint16(bh.BData[16:])
	}
	Brelse(bh)
	if s.SMagic != SUPER_MAGIC {
		s.SDev = 0; freeSuper(s); return nil
	}
	for i := 0; i < I_MAP_SLOTS; i++ { s.SImap[i] = nil }
	for i := 0; i < Z_MAP_SLOTS; i++ { s.SZmap[i] = nil }
	block := 2
	for i := 0; i < int(s.SImapBlocks); i++ {
		s.SImap[i] = Bread(dev, block)
		if s.SImap[i] != nil { block++ } else { break }
	}
	for i := 0; i < int(s.SZmapBlocks); i++ {
		s.SZmap[i] = Bread(dev, block)
		if s.SZmap[i] != nil { block++ } else { break }
	}
	if block != 2+int(s.SImapBlocks)+int(s.SZmapBlocks) {
		for i := 0; i < I_MAP_SLOTS; i++ { Brelse(s.SImap[i]) }
		for i := 0; i < Z_MAP_SLOTS; i++ { Brelse(s.SZmap[i]) }
		s.SDev = 0; freeSuper(s); return nil
	}
	s.SImap[0].BData[0] |= 1
	s.SZmap[0].BData[0] |= 1
	freeSuper(s)
	return s
}

// Namei callback (avoids circular with namei.go if it were separate pkg)
var nameiFn func(string) *MInode

func SetNamei(fn func(string) *MInode) { nameiFn = fn }

// super.c lines 167-198: sys_umount
func SysUmount(devName string) int {
	var inode *MInode
	if nameiFn != nil { inode = nameiFn(devName) }
	if inode == nil { return -ENOENT }
	dev := int(inode.IZone[0])
	if !S_ISBLK(inode.IMode) { Iput(inode); return -ENOTBLK }
	Iput(inode)
	if dev == ROOT_DEV { return -EBUSY }
	sb := GetSuper(dev)
	if sb == nil || sb.SImount == nil { return -ENOENT }
	if sb.SImount.IMount == 0 {
		log.Printf("fs: Mounted inode has i_mount=0")
	}
	for i := 0; i < NR_INODE; i++ {
		if InodeTable[i].IDev == uint16(dev) && InodeTable[i].ICount != 0 {
			return -EBUSY
		}
	}
	sb.SImount.IMount = 0
	Iput(sb.SImount)
	sb.SImount = nil
	Iput(sb.SIsup)
	sb.SIsup = nil
	PutSuper(dev)
	SyncDev(dev)
	return 0
}

// super.c lines 200-240: sys_mount
func SysMount(devName, dirName string, rwFlag int) int {
	var devI, dirI *MInode
	if nameiFn != nil { devI = nameiFn(devName) }
	if devI == nil { return -ENOENT }
	dev := int(devI.IZone[0])
	if !S_ISBLK(devI.IMode) { Iput(devI); return -EPERM }
	Iput(devI)
	if nameiFn != nil { dirI = nameiFn(dirName) }
	if dirI == nil { return -ENOENT }
	if dirI.ICount != 1 || dirI.INum == ROOT_INO {
		Iput(dirI); return -EBUSY
	}
	if !S_ISDIR(dirI.IMode) { Iput(dirI); return -EPERM }
	sb := readSuper(dev)
	if sb == nil { Iput(dirI); return -EBUSY }
	if sb.SImount != nil { Iput(dirI); return -EBUSY }
	if dirI.IMount != 0 { Iput(dirI); return -EPERM }
	sb.SImount = dirI
	dirI.IMount = 1
	dirI.IDirt = 1
	return 0
}

// File table — super.c line 250-251: file_table used in mount_root
var FileTable [NR_FILE]File

// super.c lines 242-281: mount_root
func MountRoot() {
	for i := 0; i < NR_FILE; i++ { FileTable[i].FCount = 0 }
	for i := 0; i < NR_SUPER; i++ {
		SuperBlockTable[i].SDev = 0
		SuperBlockTable[i].SLock = 0
		SuperBlockTable[i].SWait = nil
	}
	p := readSuper(ROOT_DEV)
	if p == nil { log.Printf("fs: Unable to mount root"); return }
	mi := Iget(ROOT_DEV, ROOT_INO)
	if mi == nil { log.Printf("fs: Unable to read root i-node"); return }
	mi.ICount += 3
	p.SIsup = mi
	p.SImount = mi
	Current.Pwd = mi
	Current.Root = mi
	// Count free zones
	free := 0
	for i := int(p.SNzones) - 1; i >= 0; i-- {
		if p.SZmap[i>>13] != nil && !setBit(i&8191, p.SZmap[i>>13].BData) {
			free++
		}
	}
	log.Printf("fs: %d/%d free blocks", free, p.SNzones)
	// Count free inodes
	free = 0
	for i := int(p.SNinodes); i >= 0; i-- {
		if p.SImap[i>>13] != nil && !setBit(i&8191, p.SImap[i>>13].BData) {
			free++
		}
	}
	log.Printf("fs: %d/%d free inodes", free, p.SNinodes)
}

// setBit: test and set a bit in a byte array, returns old value
func setBit(nr int, addr []byte) bool {
	if addr == nil { return true }
	byteIdx := nr >> 3
	bitIdx := uint(nr & 7)
	if byteIdx >= len(addr) { return true }
	old := (addr[byteIdx] >> bitIdx) & 1
	addr[byteIdx] |= 1 << bitIdx
	return old != 0
}

// clearBit: test and clear a bit, returns old value
func clearBit(nr int, addr []byte) bool {
	if addr == nil { return true }
	byteIdx := nr >> 3
	bitIdx := uint(nr & 7)
	if byteIdx >= len(addr) { return true }
	old := (addr[byteIdx] >> bitIdx) & 1
	addr[byteIdx] &^= 1 << bitIdx
	return old != 0
}

// findFirstZero: find first zero bit in data block
func findFirstZero(addr []byte) int {
	for i := 0; i < 8192; i++ {
		byteIdx := i >> 3
		bitIdx := uint(i & 7)
		if byteIdx >= len(addr) { return 8192 }
		if (addr[byteIdx]>>bitIdx)&1 == 0 { return i }
	}
	return 8192
}

// clearBlock: zero out a 1024-byte block
func clearBlock(addr []byte) {
	for i := range addr { addr[i] = 0 }
}
