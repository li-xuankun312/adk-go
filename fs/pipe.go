// fs/pipe.go — ported from linux-0.11/fs/pipe.c
// (C) 1991 Linus Torvalds
package fs

import (
	. "google.golang.org/adk/v2/include"
)

// Pipe data is stored in a byte buffer pointed to by inode.ISize (as uint32 address).
// In our Go port, we use a dedicated PipeData []byte per pipe inode.
// PIPE_HEAD = inode.IZone[0], PIPE_TAIL = inode.IZone[1]
// PIPE_SIZE = (HEAD - TAIL) & (PAGE_SIZE - 1)

// PipeBuffers stores per-inode pipe data (keyed by inode pointer for simplicity)
var PipeBuffers = make(map[*MInode][]byte)

func pipeSize(inode *MInode) int {
	return int((inode.IZone[0] - inode.IZone[1]) & (PAGE_SIZE - 1))
}

// pipe.c lines 13-39: read_pipe
func ReadPipe(inode *MInode, buf []byte, count int) int {
	readn := 0
	pipeBuf := PipeBuffers[inode]
	if pipeBuf == nil { return 0 }

	for count > 0 {
		for {
			size := pipeSize(inode)
			if size != 0 { break }
			if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
			if inode.ICount != 2 { return readn } // no writers
			if sleepOnFn != nil { sleepOnFn(&inode.IWait) }
		}
		size := pipeSize(inode)
		chars := PAGE_SIZE - int(inode.IZone[1]) // PAGE_SIZE - PIPE_TAIL
		if chars > count { chars = count }
		if chars > size { chars = size }
		count -= chars
		readn += chars
		tail := int(inode.IZone[1])
		inode.IZone[1] = uint16((int(inode.IZone[1]) + chars) & (PAGE_SIZE - 1))
		for i := 0; i < chars; i++ {
			if readn-chars+i < len(buf) {
				buf[readn-chars+i] = pipeBuf[tail]
			}
			tail++
		}
	}
	if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
	return readn
}

// pipe.c lines 41-69: write_pipe
func WritePipe(inode *MInode, buf []byte, count int) int {
	written := 0
	pipeBuf := PipeBuffers[inode]
	if pipeBuf == nil { return -1 }

	for count > 0 {
		for {
			size := (PAGE_SIZE - 1) - pipeSize(inode)
			if size != 0 { break }
			if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
			if inode.ICount != 2 { // no readers
				Current.Signal |= 1 << (SIGPIPE - 1)
				if written != 0 { return written }
				return -1
			}
			if sleepOnFn != nil { sleepOnFn(&inode.IWait) }
		}
		size := (PAGE_SIZE - 1) - pipeSize(inode)
		chars := PAGE_SIZE - int(inode.IZone[0]) // PAGE_SIZE - PIPE_HEAD
		if chars > count { chars = count }
		if chars > size { chars = size }
		count -= chars
		written += chars
		head := int(inode.IZone[0])
		inode.IZone[0] = uint16((int(inode.IZone[0]) + chars) & (PAGE_SIZE - 1))
		for i := 0; i < chars; i++ {
			if written-chars+i < len(buf) {
				pipeBuf[head] = buf[written-chars+i]
			}
			head++
		}
	}
	if wakeUpFn != nil { wakeUpFn(&inode.IWait) }
	return written
}

// pipe.c lines 71-111: sys_pipe
func SysPipe(fildes []int) int {
	var f [2]*File
	var fd [2]int

	// Find 2 free file table entries
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

	// Find 2 free fd slots
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
	// Allocate pipe buffer
	PipeBuffers[inode] = make([]byte, PAGE_SIZE)

	f[0].FInode = inode; f[1].FInode = inode
	f[0].FPos = 0; f[1].FPos = 0
	f[0].FMode = 1 // read
	f[1].FMode = 2 // write

	if len(fildes) >= 2 {
		fildes[0] = fd[0]
		fildes[1] = fd[1]
	}
	return 0
}
