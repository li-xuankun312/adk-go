// fs/namei.go — ported from linux-0.11/fs/namei.c
// (C) 1991 Linus Torvalds
// Some corrections by tytso.
package fs

import (
	"encoding/binary"
	"log"

	. "google.golang.org/adk/v2/include"
)

const (
	MAY_EXEC  = 1
	MAY_WRITE = 2
	MAY_READ  = 4
	O_ACCMODE = 3
	O_WRONLY  = 1
	O_EXCL    = 0200
	I_REGULAR   = 0100000
	I_DIRECTORY = 0040000
	S_ISVTX     = 01000
	EEXIST   = 17
	EISDIR   = 21
	ENOSPC   = 28
	EXDEV    = 18
	ENOTEMPTY = 39
	ERROR    = 99
)

// DirEntry size: inode(2) + name(NAME_LEN=14) = 16 bytes
const DIR_ENTRY_SIZE = 16
const DIR_ENTRIES_PER_BLOCK_C = BLOCK_SIZE / DIR_ENTRY_SIZE

// ACC_MODE: maps O_ACCMODE flags to permission bits
func accMode(flag int) int {
	tbl := [4]int{4, 2, 6, 0}
	idx := flag & O_ACCMODE
	if idx >= 4 { return 0 }
	return tbl[idx]
}

// namei.c lines 40-54: permission
func permission(inode *MInode, mask int) bool {
	mode := int(inode.IMode)
	if inode.IDev != 0 && inode.INlinks == 0 { return false }
	if Current.Euid == inode.IUid { mode >>= 6 } else if uint8(Current.Egid) == inode.IGid { mode >>= 3 }
	if ((mode & mask & 0007) == mask) || suser() { return true }
	return false
}

// namei.c lines 63-78: match — compare name with dir_entry
func matchName(length int, name string, de *DirEntry) bool {
	if de == nil || de.Inode == 0 || length > NAME_LEN { return false }
	if length < NAME_LEN && length < len(de.Name) && de.Name[length] != 0 { return false }
	for i := 0; i < length; i++ {
		if i >= len(name) || i >= len(de.Name) { return false }
		if name[i] != de.Name[i] { return false }
	}
	return true
}

// readDirEntry: read a DirEntry from buffer at given index
func readDirEntry(data []byte, offset int) *DirEntry {
	if offset+DIR_ENTRY_SIZE > len(data) { return nil }
	de := &DirEntry{}
	de.Inode = binary.LittleEndian.Uint16(data[offset:])
	copy(de.Name[:], data[offset+2:offset+DIR_ENTRY_SIZE])
	return de
}

// writeDirEntry: write a DirEntry into buffer at given offset
func writeDirEntry(data []byte, offset int, de *DirEntry) {
	if offset+DIR_ENTRY_SIZE > len(data) { return }
	binary.LittleEndian.PutUint16(data[offset:], de.Inode)
	copy(data[offset+2:offset+DIR_ENTRY_SIZE], de.Name[:])
}

// namei.c lines 91-153: find_entry
func findEntry(dir **MInode, name string, namelen int) (*BufferHead, *DirEntry, int) {
	if namelen > NAME_LEN { namelen = NAME_LEN }
	entries := int((*dir).ISize) / DIR_ENTRY_SIZE
	if namelen == 0 { return nil, nil, -1 }
	// '..' handling
	if namelen == 2 && name[0] == '.' && name[1] == '.' {
		if *dir == Current.Root { namelen = 1 }
		if (*dir).INum == ROOT_INO {
			sb := GetSuper(int((*dir).IDev))
			if sb != nil && sb.SImount != nil {
				Iput(*dir)
				*dir = sb.SImount
				(*dir).ICount++
			}
		}
	}
	if (*dir).IZone[0] == 0 { return nil, nil, -1 }
	bh := Bread(int((*dir).IDev), int((*dir).IZone[0]))
	if bh == nil { return nil, nil, -1 }
	deOffset := 0
	for i := 0; i < entries; i++ {
		if deOffset >= BLOCK_SIZE {
			Brelse(bh)
			block := Bmap(*dir, i/DIR_ENTRIES_PER_BLOCK_C)
			if block == 0 {
				i += DIR_ENTRIES_PER_BLOCK_C - 1; continue
			}
			bh = Bread(int((*dir).IDev), block)
			if bh == nil { i += DIR_ENTRIES_PER_BLOCK_C - 1; continue }
			deOffset = 0
		}
		de := readDirEntry(bh.BData, deOffset)
		if matchName(namelen, name, de) {
			return bh, de, deOffset
		}
		deOffset += DIR_ENTRY_SIZE
	}
	Brelse(bh)
	return nil, nil, -1
}

// namei.c lines 165-220: add_entry
func addEntry(dir *MInode, name string, namelen int) (*BufferHead, *DirEntry, int) {
	if namelen > NAME_LEN { namelen = NAME_LEN }
	if namelen == 0 { return nil, nil, -1 }
	if dir.IZone[0] == 0 { return nil, nil, -1 }
	bh := Bread(int(dir.IDev), int(dir.IZone[0]))
	if bh == nil { return nil, nil, -1 }
	deOffset := 0
	i := 0
	for {
		if deOffset >= BLOCK_SIZE {
			Brelse(bh)
			block := CreateBlock(dir, i/DIR_ENTRIES_PER_BLOCK_C)
			if block == 0 { return nil, nil, -1 }
			bh = Bread(int(dir.IDev), block)
			if bh == nil { i += DIR_ENTRIES_PER_BLOCK_C; continue }
			deOffset = 0
		}
		if i*DIR_ENTRY_SIZE >= int(dir.ISize) {
			// Zero the entry
			for j := 0; j < DIR_ENTRY_SIZE; j++ {
				if deOffset+j < len(bh.BData) { bh.BData[deOffset+j] = 0 }
			}
			dir.ISize = uint32((i + 1) * DIR_ENTRY_SIZE)
			dir.IDirt = 1
			dir.ICtime = uint32(CURRENT_TIME())
		}
		de := readDirEntry(bh.BData, deOffset)
		if de == nil || de.Inode == 0 {
			dir.IMtime = uint32(CURRENT_TIME())
			de = &DirEntry{}
			for j := 0; j < NAME_LEN; j++ {
				if j < namelen && j < len(name) { de.Name[j] = name[j] } else { de.Name[j] = 0 }
			}
			writeDirEntry(bh.BData, deOffset, de)
			bh.BDirt = 1
			return bh, de, deOffset
		}
		deOffset += DIR_ENTRY_SIZE
		i++
	}
}

// namei.c lines 228-270: get_dir
func getDir(pathname string) *MInode {
	if Current.Root == nil || Current.Root.ICount == 0 {
		log.Printf("fs: No root inode"); return nil
	}
	if Current.Pwd == nil || Current.Pwd.ICount == 0 {
		log.Printf("fs: No cwd inode"); return nil
	}
	if len(pathname) == 0 { return nil }
	var inode *MInode
	idx := 0
	if pathname[0] == '/' {
		inode = Current.Root; idx++
	} else {
		inode = Current.Pwd
	}
	inode.ICount++
	for idx <= len(pathname) {
		thisname := idx
		if !S_ISDIR(inode.IMode) || !permission(inode, MAY_EXEC) {
			Iput(inode); return nil
		}
		namelen := 0
		for idx < len(pathname) && pathname[idx] != '/' {
			idx++; namelen++
		}
		if idx < len(pathname) { idx++ } // skip '/'
		if namelen == 0 { return inode }
		bh, de, _ := findEntry(&inode, pathname[thisname:thisname+namelen], namelen)
		if bh == nil { Iput(inode); return nil }
		inr := int(de.Inode)
		idev := int(inode.IDev)
		Brelse(bh)
		Iput(inode)
		inode = Iget(idev, inr)
		if inode == nil { return nil }
	}
	return inode
}

// namei.c lines 278-294: dir_namei
func dirNamei(pathname string) (dir *MInode, namelen int, basename string) {
	dir = getDir(pathname)
	if dir == nil { return }
	// Find last component
	last := 0
	for i, c := range pathname {
		if c == '/' { last = i + 1 }
	}
	basename = pathname[last:]
	namelen = len(basename)
	return
}

// namei.c lines 303-330: namei
func Namei(pathname string) *MInode {
	dir, namelen, basename := dirNamei(pathname)
	if dir == nil { return nil }
	if namelen == 0 { return dir }
	bh, de, _ := findEntry(&dir, basename, namelen)
	if bh == nil { Iput(dir); return nil }
	inr := int(de.Inode)
	dev := int(dir.IDev)
	Brelse(bh)
	Iput(dir)
	inode := Iget(dev, inr)
	if inode != nil {
		inode.IAtime = uint32(CURRENT_TIME())
		inode.IDirt = 1
	}
	return inode
}

// namei.c lines 337-410: open_namei
func OpenNamei(pathname string, flag, mode int, resInode **MInode) int {
	if (flag&O_TRUNC) != 0 && (flag&O_ACCMODE) == 0 { flag |= O_WRONLY }
	mode &= 0777 & ^int(Current.Umask)
	mode |= I_REGULAR
	dir, namelen, basename := dirNamei(pathname)
	if dir == nil { return -ENOENT }
	if namelen == 0 {
		if flag&(O_ACCMODE|O_CREAT|O_TRUNC) == 0 { *resInode = dir; return 0 }
		Iput(dir); return -EISDIR
	}
	bh, de, _ := findEntry(&dir, basename, namelen)
	if bh == nil {
		if flag&O_CREAT == 0 { Iput(dir); return -ENOENT }
		if !permission(dir, MAY_WRITE) { Iput(dir); return -EACCES }
		inode := NewInode(int(dir.IDev))
		if inode == nil { Iput(dir); return -ENOSPC }
		inode.IUid = Current.Euid
		inode.IMode = uint16(mode)
		inode.IDirt = 1
		bh2, de2, _ := addEntry(dir, basename, namelen)
		if bh2 == nil {
			inode.INlinks--; Iput(inode); Iput(dir); return -ENOSPC
		}
		de2.Inode = inode.INum
		writeDirEntry(bh2.BData, 0, de2) // re-write inode number
		bh2.BDirt = 1; Brelse(bh2); Iput(dir)
		*resInode = inode; return 0
	}
	inr := int(de.Inode)
	dev := int(dir.IDev)
	Brelse(bh); Iput(dir)
	if flag&O_EXCL != 0 { return -EEXIST }
	inode := Iget(dev, inr)
	if inode == nil { return -EACCES }
	if (S_ISDIR(inode.IMode) && (flag&O_ACCMODE) != 0) ||
		!permission(inode, accMode(flag)) {
		Iput(inode); return -EPERM
	}
	inode.IAtime = uint32(CURRENT_TIME())
	if flag&O_TRUNC != 0 { Truncate(inode) }
	*resInode = inode
	return 0
}

// namei.c lines 412-461: sys_mknod
func SysMknod(filename string, mode, dev int) int {
	if !suser() { return -EPERM }
	dir, namelen, basename := dirNamei(filename)
	if dir == nil { return -ENOENT }
	if namelen == 0 { Iput(dir); return -ENOENT }
	if !permission(dir, MAY_WRITE) { Iput(dir); return -EPERM }
	bh, _, _ := findEntry(&dir, basename, namelen)
	if bh != nil { Brelse(bh); Iput(dir); return -EEXIST }
	inode := NewInode(int(dir.IDev))
	if inode == nil { Iput(dir); return -ENOSPC }
	inode.IMode = uint16(mode)
	if S_ISBLK(uint16(mode)) || S_ISCHR(uint16(mode)) { inode.IZone[0] = uint16(dev) }
	ct := uint32(CURRENT_TIME())
	inode.IMtime = ct; inode.IAtime = ct; inode.IDirt = 1
	bh2, de2, _ := addEntry(dir, basename, namelen)
	if bh2 == nil { Iput(dir); inode.INlinks = 0; Iput(inode); return -ENOSPC }
	de2.Inode = inode.INum
	writeDirEntry(bh2.BData, 0, de2)
	bh2.BDirt = 1; Iput(dir); Iput(inode); Brelse(bh2)
	return 0
}

// namei.c lines 463-538: sys_mkdir
func SysMkdir(pathname string, mode int) int {
	if !suser() { return -EPERM }
	dir, namelen, basename := dirNamei(pathname)
	if dir == nil { return -ENOENT }
	if namelen == 0 { Iput(dir); return -ENOENT }
	if !permission(dir, MAY_WRITE) { Iput(dir); return -EPERM }
	bh, _, _ := findEntry(&dir, basename, namelen)
	if bh != nil { Brelse(bh); Iput(dir); return -EEXIST }
	inode := NewInode(int(dir.IDev))
	if inode == nil { Iput(dir); return -ENOSPC }
	inode.ISize = 32
	inode.IDirt = 1
	ct := uint32(CURRENT_TIME())
	inode.IMtime = ct; inode.IAtime = ct
	nb := NewBlock(int(inode.IDev))
	if nb == 0 { Iput(dir); inode.INlinks--; Iput(inode); return -ENOSPC }
	inode.IZone[0] = uint16(nb)
	inode.IDirt = 1
	dirBlock := Bread(int(inode.IDev), nb)
	if dirBlock == nil {
		Iput(dir); FreeBlock(int(inode.IDev), nb)
		inode.INlinks--; Iput(inode); return -ERROR
	}
	// Write '.' entry
	dot := &DirEntry{Inode: inode.INum}
	dot.Name[0] = '.'
	writeDirEntry(dirBlock.BData, 0, dot)
	// Write '..' entry
	dotdot := &DirEntry{Inode: dir.INum}
	dotdot.Name[0] = '.'; dotdot.Name[1] = '.'
	writeDirEntry(dirBlock.BData, DIR_ENTRY_SIZE, dotdot)
	inode.INlinks = 2
	dirBlock.BDirt = 1
	Brelse(dirBlock)
	inode.IMode = uint16(I_DIRECTORY | (mode & 0777 & ^int(Current.Umask)))
	inode.IDirt = 1
	bh2, de2, _ := addEntry(dir, basename, namelen)
	if bh2 == nil {
		Iput(dir); FreeBlock(int(inode.IDev), int(inode.IZone[0]))
		inode.INlinks = 0; Iput(inode); return -ENOSPC
	}
	de2.Inode = inode.INum
	writeDirEntry(bh2.BData, 0, de2)
	bh2.BDirt = 1
	dir.INlinks++; dir.IDirt = 1
	Iput(dir); Iput(inode); Brelse(bh2)
	return 0
}

// namei.c lines 543-585: empty_dir
func emptyDir(inode *MInode) bool {
	nEntries := int(inode.ISize) / DIR_ENTRY_SIZE
	if nEntries < 2 || inode.IZone[0] == 0 { return false }
	bh := Bread(int(inode.IDev), int(inode.IZone[0]))
	if bh == nil { return false }
	de0 := readDirEntry(bh.BData, 0)
	de1 := readDirEntry(bh.BData, DIR_ENTRY_SIZE)
	if de0 == nil || de1 == nil || de0.Inode != inode.INum || de1.Inode == 0 {
		Brelse(bh); return false
	}
	deOffset := 2 * DIR_ENTRY_SIZE
	for nr := 2; nr < nEntries; nr++ {
		if deOffset >= BLOCK_SIZE {
			Brelse(bh)
			block := Bmap(inode, nr/DIR_ENTRIES_PER_BLOCK_C)
			if block == 0 { nr += DIR_ENTRIES_PER_BLOCK_C - 1; continue }
			bh = Bread(int(inode.IDev), block)
			if bh == nil { return false }
			deOffset = 0
		}
		de := readDirEntry(bh.BData, deOffset)
		if de != nil && de.Inode != 0 { Brelse(bh); return false }
		deOffset += DIR_ENTRY_SIZE
	}
	Brelse(bh)
	return true
}

// namei.c lines 587-661: sys_rmdir
func SysRmdir(name string) int {
	if !suser() { return -EPERM }
	dir, namelen, basename := dirNamei(name)
	if dir == nil { return -ENOENT }
	if namelen == 0 { Iput(dir); return -ENOENT }
	if !permission(dir, MAY_WRITE) { Iput(dir); return -EPERM }
	bh, de, deOff := findEntry(&dir, basename, namelen)
	if bh == nil { Iput(dir); return -ENOENT }
	inode := Iget(int(dir.IDev), int(de.Inode))
	if inode == nil { Iput(dir); Brelse(bh); return -EPERM }
	if (dir.IMode&S_ISVTX) != 0 && Current.Euid != 0 && inode.IUid != Current.Euid {
		Iput(dir); Iput(inode); Brelse(bh); return -EPERM
	}
	if inode.IDev != dir.IDev || inode.ICount > 1 {
		Iput(dir); Iput(inode); Brelse(bh); return -EPERM
	}
	if inode == dir { Iput(inode); Iput(dir); Brelse(bh); return -EPERM }
	if !S_ISDIR(inode.IMode) { Iput(inode); Iput(dir); Brelse(bh); return -ENOTDIR }
	if !emptyDir(inode) { Iput(inode); Iput(dir); Brelse(bh); return -ENOTEMPTY }
	// Zero the dir entry
	binary.LittleEndian.PutUint16(bh.BData[deOff:], 0)
	bh.BDirt = 1; Brelse(bh)
	inode.INlinks = 0; inode.IDirt = 1
	dir.INlinks--
	ct := uint32(CURRENT_TIME())
	dir.ICtime = ct; dir.IMtime = ct; dir.IDirt = 1
	Iput(dir); Iput(inode)
	return 0
}

// namei.c lines 663-719: sys_unlink
func SysUnlink(name string) int {
	dir, namelen, basename := dirNamei(name)
	if dir == nil { return -ENOENT }
	if namelen == 0 { Iput(dir); return -ENOENT }
	if !permission(dir, MAY_WRITE) { Iput(dir); return -EPERM }
	bh, de, deOff := findEntry(&dir, basename, namelen)
	if bh == nil { Iput(dir); return -ENOENT }
	inode := Iget(int(dir.IDev), int(de.Inode))
	if inode == nil { Iput(dir); Brelse(bh); return -ENOENT }
	if (dir.IMode&S_ISVTX) != 0 && !suser() &&
		Current.Euid != inode.IUid && Current.Euid != dir.IUid {
		Iput(dir); Iput(inode); Brelse(bh); return -EPERM
	}
	if S_ISDIR(inode.IMode) { Iput(inode); Iput(dir); Brelse(bh); return -EPERM }
	if inode.INlinks == 0 {
		log.Printf("fs: Deleting nonexistent file (%04x:%d), %d",
			inode.IDev, inode.INum, inode.INlinks)
		inode.INlinks = 1
	}
	binary.LittleEndian.PutUint16(bh.BData[deOff:], 0)
	bh.BDirt = 1; Brelse(bh)
	inode.INlinks--; inode.IDirt = 1
	inode.ICtime = uint32(CURRENT_TIME())
	Iput(inode); Iput(dir)
	return 0
}

// namei.c lines 721-778: sys_link
func SysLink(oldname, newname string) int {
	oldinode := Namei(oldname)
	if oldinode == nil { return -ENOENT }
	if S_ISDIR(oldinode.IMode) { Iput(oldinode); return -EPERM }
	dir, namelen, basename := dirNamei(newname)
	if dir == nil { Iput(oldinode); return -EACCES }
	if namelen == 0 { Iput(oldinode); Iput(dir); return -EPERM }
	if dir.IDev != oldinode.IDev { Iput(dir); Iput(oldinode); return -EXDEV }
	if !permission(dir, MAY_WRITE) { Iput(dir); Iput(oldinode); return -EACCES }
	bh, _, _ := findEntry(&dir, basename, namelen)
	if bh != nil { Brelse(bh); Iput(dir); Iput(oldinode); return -EEXIST }
	bh2, de2, _ := addEntry(dir, basename, namelen)
	if bh2 == nil { Iput(dir); Iput(oldinode); return -ENOSPC }
	de2.Inode = oldinode.INum
	writeDirEntry(bh2.BData, 0, de2)
	bh2.BDirt = 1; Brelse(bh2); Iput(dir)
	oldinode.INlinks++
	oldinode.ICtime = uint32(CURRENT_TIME())
	oldinode.IDirt = 1
	Iput(oldinode)
	return 0
}
