// blk_drv/ll_rw_blk.go — ported from linux-0.11/kernel/blk_drv/ll_rw_blk.c
// (C) 1991 Linus Torvalds
//
// This handles all read/write requests to block devices
package blk_drv

import (
	"log"
	"sync"

	. "google.golang.org/adk/v2/include"
)

// blk.h constants
const (
	NR_BLK_DEV = 7
	NR_REQUEST = 32
	WRITEA     = 3 // write-ahead
)

// blk.h lines 23-33: struct request
type Request struct {
	Dev       int             // -1 if no request
	Cmd       int             // READ or WRITE
	Errors    int
	Sector    uint32
	NrSectors uint32
	Buffer    []byte
	Waiting   *TaskStruct
	Bh        *BufferHead
	Next      *Request
}

// blk.h lines 45-48: struct blk_dev_struct
type BlkDevStruct struct {
	RequestFn      func()   // do_request handler
	CurrentRequest *Request
}

// ll_rw_blk.c lines 21-40: globals
var (
	Requests       [NR_REQUEST]Request
	WaitForRequest *TaskStruct
	BlkDev         [NR_BLK_DEV]BlkDevStruct
	blkMu          sync.Mutex
)

// Kernel callbacks
var (
	sleepOnFn func(**TaskStruct)
	wakeUpFn  func(**TaskStruct)
)

func SetSleepOn(fn func(**TaskStruct)) { sleepOnFn = fn }
func SetWakeUp(fn func(**TaskStruct))  { wakeUpFn = fn }

// ll_rw_blk.c lines 42-49: lock_buffer
func lockBuffer(bh *BufferHead) {
	blkMu.Lock()
	for bh.BLock != 0 {
		blkMu.Unlock()
		if sleepOnFn != nil { sleepOnFn(&bh.BWait) }
		blkMu.Lock()
	}
	bh.BLock = 1
	blkMu.Unlock()
}

// ll_rw_blk.c lines 51-57: unlock_buffer
func unlockBuffer(bh *BufferHead) {
	if bh.BLock == 0 {
		log.Printf("blk_drv: buffer not locked")
	}
	bh.BLock = 0
	if wakeUpFn != nil { wakeUpFn(&bh.BWait) }
}

// IN_ORDER: elevator algorithm comparator
func inOrder(s1, s2 *Request) bool {
	if s1.Cmd < s2.Cmd { return true }
	if s1.Cmd == s2.Cmd {
		if s1.Dev < s2.Dev { return true }
		if s1.Dev == s2.Dev && s1.Sector < s2.Sector { return true }
	}
	return false
}

// ll_rw_blk.c lines 64-86: add_request
func addRequest(dev *BlkDevStruct, req *Request) {
	req.Next = nil
	blkMu.Lock()
	if req.Bh != nil { req.Bh.BDirt = 0 }
	if dev.CurrentRequest == nil {
		dev.CurrentRequest = req
		blkMu.Unlock()
		if dev.RequestFn != nil { dev.RequestFn() }
		return
	}
	tmp := dev.CurrentRequest
	for tmp.Next != nil {
		if (inOrder(tmp, req) || !inOrder(tmp, tmp.Next)) && inOrder(req, tmp.Next) {
			break
		}
		tmp = tmp.Next
	}
	req.Next = tmp.Next
	tmp.Next = req
	blkMu.Unlock()
}

// ll_rw_blk.c lines 88-143: make_request
func makeRequest(major, rw int, bh *BufferHead) {
	rwAhead := (rw == READA || rw == WRITEA)
	if rwAhead {
		if bh.BLock != 0 { return }
		if rw == READA { rw = READ } else { rw = WRITE }
	}
	if rw != READ && rw != WRITE {
		log.Printf("blk_drv: Bad block dev command, must be R/W/RA/WA")
		return
	}
	lockBuffer(bh)
	if (rw == WRITE && bh.BDirt == 0) || (rw == READ && bh.BUptodate != 0) {
		unlockBuffer(bh)
		return
	}
repeat:
	// Find an empty request
	var req *Request
	limit := NR_REQUEST
	if rw != READ { limit = (NR_REQUEST * 2) / 3 }
	for i := limit - 1; i >= 0; i-- {
		if Requests[i].Dev < 0 {
			req = &Requests[i]
			break
		}
	}
	if req == nil {
		if rwAhead { unlockBuffer(bh); return }
		if sleepOnFn != nil { sleepOnFn(&WaitForRequest) }
		goto repeat
	}
	req.Dev = int(bh.BDev)
	req.Cmd = rw
	req.Errors = 0
	req.Sector = bh.BBlocknr << 1
	req.NrSectors = 2
	req.Buffer = bh.BData
	req.Waiting = nil
	req.Bh = bh
	req.Next = nil
	addRequest(&BlkDev[major], req)
}

// ll_rw_blk.c lines 145-155: ll_rw_block
func LlRwBlock(rw int, bh *BufferHead) {
	major := int(MAJOR(uint32(bh.BDev)))
	if major >= NR_BLK_DEV || BlkDev[major].RequestFn == nil {
		log.Printf("blk_drv: Trying to read nonexistent block-device")
		return
	}
	makeRequest(major, rw, bh)
}

// ll_rw_blk.c lines 157-165: blk_dev_init
func BlkDevInit() {
	for i := 0; i < NR_REQUEST; i++ {
		Requests[i].Dev = -1
		Requests[i].Next = nil
	}
}

// end_request — used by device drivers
func EndRequest(uptodate int) {
	dev := BlkDev // placeholder; real code indexes by MAJOR_NR
	_ = dev
	// Generic: find current request from the right device
	// In practice each driver calls this with its own MAJOR
}

// EndRequestForDev — driver calls this to complete a request
func EndRequestForDev(major int, uptodate int) {
	req := BlkDev[major].CurrentRequest
	if req == nil { return }
	if req.Bh != nil {
		if uptodate != 0 {
			req.Bh.BUptodate = 1
		} else {
			req.Bh.BUptodate = 0
		}
		unlockBuffer(req.Bh)
	}
	if uptodate == 0 {
		log.Printf("blk_drv: I/O error, dev %04x, block %d", req.Dev, req.Bh.BBlocknr)
	}
	if wakeUpFn != nil {
		wakeUpFn(&req.Waiting)
		wakeUpFn(&WaitForRequest)
	}
	req.Dev = -1
	BlkDev[major].CurrentRequest = req.Next
}
