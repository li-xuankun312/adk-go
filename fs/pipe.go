// fs/pipe.go — ported from linux-0.11/fs/pipe.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

// PIPE macros from include/linux/fs.h
// PIPE_HEAD(inode) = inode.i_zone[0]
// PIPE_TAIL(inode) = inode.i_zone[1]
// PIPE_SIZE(inode) = ((PIPE_HEAD - PIPE_TAIL) & (PAGE_SIZE-1))

func pipeHead(inode *MInode) int { return int(inode.IZone[0]) }
func pipeTail(inode *MInode) int { return int(inode.IZone[1]) }
func pipeSize(inode *MInode) int { return (pipeHead(inode) - pipeTail(inode)) & (PAGE_SIZE - 1) }

func setPipeHead(inode *MInode, v int) { inode.IZone[0] = uint16(v) }
func setPipeTail(inode *MInode, v int) { inode.IZone[1] = uint16(v) }

// Pipe data buffer: in original, inode->i_size points to a page.
// We use a byte-slice stored as PipeData field (added to MInode).
// For compatibility, we use inode.ISize as the starting address (simulated),
// but actually store data in a separate pipeData map.
var pipeDataMap = make(map[*MInode][]byte)

func getPipeData(inode *MInode) []byte {
	d := pipeDataMap[inode]
	if d == nil {
		d = make([]byte, PAGE_SIZE)
		pipeDataMap[inode] = d
	}
	return d
}

// pipe.c lines 13-39: read_pipe
func ReadPipe(inode *MInode, buf []byte, count int) int {
	readBytes := 0
	for count > 0 {
		for pipeSize(inode) == 0 {
			if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
			if inode.ICount != 2 { return readBytes }
			if sleepOnFn != nil { sleepOnFn(&inode.IWait) }
		}
		chars := PAGE_SIZE - pipeTail(inode)
		if chars > count { chars = count }
		if chars > pipeSize(inode) { chars = pipeSize(inode) }
		count -= chars
		readBytes += chars
		data := getPipeData(inode)
		tail := pipeTail(inode)
		for i := 0; i < chars; i++ {
			if readBytes-chars+i < len(buf) {
				buf[readBytes-chars+i] = data[tail]
			}
			tail++
		}
		setPipeTail(inode, (pipeTail(inode)+chars)&(PAGE_SIZE-1))
	}
	if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
	return readBytes
}

// pipe.c lines 41-69: write_pipe
func WritePipe(inode *MInode, buf []byte, count int) int {
	written := 0
	for count > 0 {
		for (PAGE_SIZE-1)-pipeSize(inode) == 0 {
			if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
			if inode.ICount != 2 {
				Current.Signal |= 1 << (SIGPIPE - 1)
				if written != 0 { return written }
				return -1
			}
			if sleepOnFn != nil { sleepOnFn(&inode.IWait) }
		}
		chars := PAGE_SIZE - pipeHead(inode)
		size := (PAGE_SIZE - 1) - pipeSize(inode)
		if chars > count { chars = count }
		if chars > size { chars = size }
		count -= chars
		written += chars
		data := getPipeData(inode)
		head := pipeHead(inode)
		for i := 0; i < chars; i++ {
			if written-chars+i < len(buf) {
				data[head] = buf[written-chars+i]
			}
			head++
		}
		setPipeHead(inode, (pipeHead(inode)+chars)&(PAGE_SIZE-1))
	}
	if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
	return written
}

// pipe.c lines 71-111: sys_pipe
func SysPipe(fildes *[2]int) int {
	var f [2]*File
	j := 0
	for i := 0; j < 2 && i < NR_FILE; i++ {
		if FileTable[i].FCount == 0 {
			f[j] = &FileTable[i]
			f[j].FCount++
			j++
		}
	}
	if j == 1 { f[0].FCount = 0 }
	if j < 2 { return -1 }
	var fd [2]int
	j = 0
	for i := 0; j < 2 && i < NR_OPEN; i++ {
		if Current.Filp[i] == nil {
			fd[j] = i
			Current.Filp[i] = f[j]
			j++
		}
	}
	if j == 1 { Current.Filp[fd[0]] = nil }
	if j < 2 {
		f[0].FCount = 0; f[1].FCount = 0
		return -1
	}
	inode := GetPipeInode()
	if inode == nil {
		Current.Filp[fd[0]] = nil
		Current.Filp[fd[1]] = nil
		f[0].FCount = 0; f[1].FCount = 0
		return -1
	}
	f[0].FInode = inode; f[1].FInode = inode
	f[0].FPos = 0; f[1].FPos = 0
	f[0].FMode = 1 // read
	f[1].FMode = 2 // write
	fildes[0] = fd[0]
	fildes[1] = fd[1]
	return 0
}
