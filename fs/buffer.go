// fs/buffer.go — ported from linux-0.11/fs/buffer.c
// (C) 1991 Linus Torvalds
//
// 'buffer.c' implements the buffer-cache functions. Race-conditions have
// been avoided by NEVER letting an interrupt change a buffer (except for the
// data, of course), but instead letting the caller do it.
package fs

import (
	"log"
	"sync"

	. "google.golang.org/adk/v2/include"
)

// buffer.c lines 33-37: globals
var (
	StartBuffer *BufferHead              // struct buffer_head * start_buffer
	hashTable   [NR_HASH]*BufferHead     // struct buffer_head * hash_table[NR_HASH]
	freeList    *BufferHead              // static struct buffer_head * free_list
	bufferWait  *TaskStruct              // static struct task_struct * buffer_wait
	NrBuffers   int                      // int NR_BUFFERS
	buffers     []BufferHead             // pre-allocated buffer head array
	bufferData  []byte                   // backing storage for buffer data blocks
	bufMu       sync.Mutex               // replaces cli/sti
)

// Kernel callback functions (avoids circular import)
var (
	sleepOnFn  func(**TaskStruct)
	wakeUpFn   func(**TaskStruct)
	llRwBlockFn func(int, *BufferHead)
	syncInodesFn func()
	putSuperFn   func(int)
	invalidateInodesFn func(int)
)

func SetSleepOn(fn func(**TaskStruct))          { sleepOnFn = fn }
func SetWakeUp(fn func(**TaskStruct))            { wakeUpFn = fn }
func SetLlRwBlock(fn func(int, *BufferHead))     { llRwBlockFn = fn }
func SetSyncInodes(fn func())                    { syncInodesFn = fn }
func SetPutSuper(fn func(int))                   { putSuperFn = fn }
func SetInvalidateInodes(fn func(int))           { invalidateInodesFn = fn }

// buffer.c lines 39-45: wait_on_buffer
func waitOnBuffer(bh *BufferHead) {
	bufMu.Lock()
	for bh.BLock != 0 {
		bufMu.Unlock()
		if sleepOnFn != nil {
			sleepOnFn(&bh.BWait)
		}
		bufMu.Lock()
	}
	bufMu.Unlock()
}

// buffer.c lines 47-60: sys_sync
func SysSync() int {
	SyncInodes()
	for i := 0; i < NrBuffers; i++ {
		bh := &buffers[i]
		waitOnBuffer(bh)
		if bh.BDirt != 0 {
			llRwBlock(WRITE, bh)
		}
	}
	return 0
}

// buffer.c lines 62-85: sync_dev
func SyncDev(dev int) int {
	for i := 0; i < NrBuffers; i++ {
		bh := &buffers[i]
		if bh.BDev != uint16(dev) { continue }
		waitOnBuffer(bh)
		if bh.BDev == uint16(dev) && bh.BDirt != 0 {
			llRwBlock(WRITE, bh)
		}
	}
	SyncInodes()
	for i := 0; i < NrBuffers; i++ {
		bh := &buffers[i]
		if bh.BDev != uint16(dev) { continue }
		waitOnBuffer(bh)
		if bh.BDev == uint16(dev) && bh.BDirt != 0 {
			llRwBlock(WRITE, bh)
		}
	}
	return 0
}

// buffer.c lines 87-100: invalidate_buffers
func invalidateBuffers(dev int) {
	for i := 0; i < NrBuffers; i++ {
		bh := &buffers[i]
		if bh.BDev != uint16(dev) { continue }
		waitOnBuffer(bh)
		if bh.BDev == uint16(dev) {
			bh.BUptodate = 0
			bh.BDirt = 0
		}
	}
}

// buffer.c lines 116-129: check_disk_change
func CheckDiskChange(dev int) {
	if MAJOR(uint32(dev)) != 2 { return }
	// floppy_change — stubbed for Go port
	invalidateBuffers(dev)
}

// buffer.c lines 131-132: hash function
func hashfn(dev, block int) int {
	return int(uint(dev^block) % NR_HASH)
}

// buffer.c lines 134-150: remove_from_queues
func removeFromQueues(bh *BufferHead) {
	if bh.BNext != nil { bh.BNext.BPrev = bh.BPrev }
	if bh.BPrev != nil { bh.BPrev.BNext = bh.BNext }
	if hashTable[hashfn(int(bh.BDev), int(bh.BBlocknr))] == bh {
		hashTable[hashfn(int(bh.BDev), int(bh.BBlocknr))] = bh.BNext
	}
	if bh.BPrevFree == nil || bh.BNextFree == nil {
		log.Printf("fs: Free block list corrupted")
		return
	}
	bh.BPrevFree.BNextFree = bh.BNextFree
	bh.BNextFree.BPrevFree = bh.BPrevFree
	if freeList == bh { freeList = bh.BNextFree }
}

// buffer.c lines 152-167: insert_into_queues
func insertIntoQueues(bh *BufferHead) {
	bh.BNextFree = freeList
	bh.BPrevFree = freeList.BPrevFree
	freeList.BPrevFree.BNextFree = bh
	freeList.BPrevFree = bh
	bh.BPrev = nil
	bh.BNext = nil
	if bh.BDev == 0 { return }
	bh.BNext = hashTable[hashfn(int(bh.BDev), int(bh.BBlocknr))]
	hashTable[hashfn(int(bh.BDev), int(bh.BBlocknr))] = bh
	if bh.BNext != nil { bh.BNext.BPrev = bh }
}

// buffer.c lines 169-177: find_buffer
func findBuffer(dev, block int) *BufferHead {
	for tmp := hashTable[hashfn(dev, block)]; tmp != nil; tmp = tmp.BNext {
		if tmp.BDev == uint16(dev) && tmp.BBlocknr == uint32(block) {
			return tmp
		}
	}
	return nil
}

// buffer.c lines 186-199: get_hash_table
func GetHashTable(dev, block int) *BufferHead {
	for {
		bh := findBuffer(dev, block)
		if bh == nil { return nil }
		bh.BCount++
		waitOnBuffer(bh)
		if bh.BDev == uint16(dev) && bh.BBlocknr == uint32(block) {
			return bh
		}
		bh.BCount--
	}
}

// buffer.c line 208
func badness(bh *BufferHead) int { return int(bh.BDirt)<<1 + int(bh.BLock) }

// buffer.c lines 209-254: getblk
func Getblk(dev, block int) *BufferHead {
repeat:
	if bh := GetHashTable(dev, block); bh != nil { return bh }
	tmp := freeList
	var bh *BufferHead
	for {
		if tmp.BCount == 0 {
			if bh == nil || badness(tmp) < badness(bh) {
				bh = tmp
				if badness(tmp) == 0 { break }
			}
		}
		tmp = tmp.BNextFree
		if tmp == freeList { break }
	}
	if bh == nil {
		if sleepOnFn != nil { sleepOnFn(&bufferWait) }
		goto repeat
	}
	waitOnBuffer(bh)
	if bh.BCount != 0 { goto repeat }
	for bh.BDirt != 0 {
		SyncDev(int(bh.BDev))
		waitOnBuffer(bh)
		if bh.BCount != 0 { goto repeat }
	}
	if findBuffer(dev, block) != nil { goto repeat }
	bh.BCount = 1
	bh.BDirt = 0
	bh.BUptodate = 0
	removeFromQueues(bh)
	bh.BDev = uint16(dev)
	bh.BBlocknr = uint32(block)
	insertIntoQueues(bh)
	return bh
}

// buffer.c lines 256-264: brelse
func Brelse(buf *BufferHead) {
	if buf == nil { return }
	waitOnBuffer(buf)
	if buf.BCount == 0 {
		log.Printf("fs: Trying to free free buffer")
		return
	}
	buf.BCount--
	if wakeUpFn != nil { wakeUpFn(&bufferWait) }
}

// buffer.c lines 270-284: bread
func Bread(dev, block int) *BufferHead {
	bh := Getblk(dev, block)
	if bh == nil { log.Printf("fs: bread: getblk returned NULL"); return nil }
	if bh.BUptodate != 0 { return bh }
	llRwBlock(READ, bh)
	waitOnBuffer(bh)
	if bh.BUptodate != 0 { return bh }
	Brelse(bh)
	return nil
}

// buffer.c lines 299-318: bread_page
func BreadPage(address uint32, dev int, b [4]int) {
	var bh [4]*BufferHead
	for i := 0; i < 4; i++ {
		if b[i] != 0 {
			bh[i] = Getblk(dev, b[i])
			if bh[i] != nil && bh[i].BUptodate == 0 { llRwBlock(READ, bh[i]) }
		}
	}
	for i := 0; i < 4; i++ {
		if bh[i] != nil {
			waitOnBuffer(bh[i])
			// COPYBLK(bh[i]->b_data, address) — copy to physmem
			Brelse(bh[i])
		}
		address += BLOCK_SIZE
	}
}

// buffer.c lines 325-349: breada
func Breada(dev, first int, blocks ...int) *BufferHead {
	bh := Getblk(dev, first)
	if bh == nil { log.Printf("fs: breada: getblk returned NULL"); return nil }
	if bh.BUptodate == 0 { llRwBlock(READ, bh) }
	for _, b := range blocks {
		if b < 0 { break }
		tmp := Getblk(dev, b)
		if tmp != nil {
			if tmp.BUptodate == 0 { llRwBlock(READA, tmp) }
			tmp.BCount--
		}
	}
	waitOnBuffer(bh)
	if bh.BUptodate != 0 { return bh }
	Brelse(bh)
	return nil
}

// llRwBlock wrapper
func llRwBlock(rw int, bh *BufferHead) {
	if llRwBlockFn != nil { llRwBlockFn(rw, bh) }
}

// buffer.c lines 351-384: buffer_init
func BufferInit(bufferEnd int32) {
	numBuffers := 256
	if bufferEnd > 0 {
		numBuffers = int(bufferEnd) / (BLOCK_SIZE + 64)
		if numBuffers < 32 { numBuffers = 32 }
		if numBuffers > 4096 { numBuffers = 4096 }
	}
	buffers = make([]BufferHead, numBuffers)
	bufferData = make([]byte, numBuffers*BLOCK_SIZE)
	for i := 0; i < numBuffers; i++ {
		h := &buffers[i]
		h.BDev = 0; h.BDirt = 0; h.BCount = 0; h.BLock = 0
		h.BUptodate = 0; h.BWait = nil; h.BNext = nil; h.BPrev = nil
		h.BData = bufferData[i*BLOCK_SIZE : (i+1)*BLOCK_SIZE]
		if i > 0 { h.BPrevFree = &buffers[i-1] }
		if i < numBuffers-1 { h.BNextFree = &buffers[i+1] }
	}
	buffers[0].BPrevFree = &buffers[numBuffers-1]
	buffers[numBuffers-1].BNextFree = &buffers[0]
	freeList = &buffers[0]
	StartBuffer = &buffers[0]
	NrBuffers = numBuffers
	for i := 0; i < NR_HASH; i++ { hashTable[i] = nil }
	log.Printf("fs: buffer_init: %d buffers", NrBuffers)
}
