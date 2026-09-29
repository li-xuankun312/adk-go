// blk_drv/hd.go — ported from linux-0.11/kernel/blk_drv/hd.c
// (C) 1991 Linus Torvalds
//
// Hard disk driver. In the Go port, actual I/O is simulated via
// an in-memory disk image (or file-backed store).
package blk_drv

import (
	"log"

	. "google.golang.org/adk/v2/include"
)

const (
	MAJOR_NR_HD = 3
	MAX_ERRORS  = 7
	MAX_HD      = 2
)

// hd.c lines 45-47: struct hd_i_struct
type HdInfoStruct struct {
	Head, Sect, Cyl, Wpcom, Lzone, Ctl int
}

// hd.c lines 56-59: struct hd_struct
type HdStruct struct {
	StartSect int32
	NrSects   int32
}

var (
	HdInfo    [MAX_HD]HdInfoStruct
	Hd        [5 * MAX_HD]HdStruct
	NrHd      int
	recalibrate int
	reset       int
)

// Simulated disk storage
var DiskImages [MAX_HD][]byte

// hd.c lines 71-155: sys_setup — read partition table
func SysSetup() int {
	// In Go, disk geometry is configured programmatically
	for i := 0; i < NrHd; i++ {
		Hd[i*5].StartSect = 0
		Hd[i*5].NrSects = int32(HdInfo[i].Head * HdInfo[i].Sect * HdInfo[i].Cyl)
	}
	// Read partition tables from first sector of each drive
	for drive := 0; drive < NrHd; drive++ {
		bh := Bread_blk(int(0x300+drive*5), 0)
		if bh == nil {
			log.Printf("hd: unable to read partition table of drive %d", drive)
			continue
		}
		// Partition table at offset 0x1BE in sector 0
		if len(bh.BData) >= 0x1FE {
			// Validate 0x55AA signature
			if bh.BData[0x1FE] == 0x55 && bh.BData[0x1FF] == 0xAA {
				for i := 0; i < 4; i++ {
					off := 0x1BE + i*16
					p := &Hd[drive*5+1+i]
					p.StartSect = int32(uint32(bh.BData[off+8]) |
						uint32(bh.BData[off+9])<<8 |
						uint32(bh.BData[off+10])<<16 |
						uint32(bh.BData[off+11])<<24)
					p.NrSects = int32(uint32(bh.BData[off+12]) |
						uint32(bh.BData[off+13])<<8 |
						uint32(bh.BData[off+14])<<16 |
						uint32(bh.BData[off+15])<<24)
				}
			}
		}
		Brelse_blk(bh)
	}
	log.Printf("hd: partition table(s) ok.")
	return 0
}

// Bread/Brelse wrappers (via function variables to avoid circular import)
var (
	breadFn  func(int, int) *BufferHead
	brelseFn func(*BufferHead)
)

func SetBread(fn func(int, int) *BufferHead) { breadFn = fn }
func SetBrelse(fn func(*BufferHead))          { brelseFn = fn }

func Bread_blk(dev, block int) *BufferHead {
	if breadFn != nil { return breadFn(dev, block) }
	return nil
}

func Brelse_blk(bh *BufferHead) {
	if brelseFn != nil { brelseFn(bh) }
}

// hd.c: do_hd_request — process the current request
func DoHdRequest() {
	req := BlkDev[MAJOR_NR_HD].CurrentRequest
	if req == nil { return }
	if int(MAJOR(uint32(req.Dev))) != MAJOR_NR_HD {
		log.Printf("hd: request list destroyed")
		return
	}
	// Determine drive, head, sector, cylinder from request
	dev := int(MINOR(uint32(req.Dev)))
	drive := dev / 5
	if drive >= NrHd || drive >= MAX_HD {
		EndRequestForDev(MAJOR_NR_HD, 0)
		return
	}
	// Calculate physical sector
	blockNr := int(req.Sector)
	startSect := int(Hd[dev].StartSect)
	nrSects := int(Hd[dev].NrSects)
	if blockNr+2 > nrSects {
		EndRequestForDev(MAJOR_NR_HD, 0)
		return
	}
	sector := blockNr + startSect
	// Simulate I/O from DiskImages
	if drive < len(DiskImages) && DiskImages[drive] != nil {
		byteOff := sector * 512
		if req.Cmd == READ {
			if byteOff+1024 <= len(DiskImages[drive]) && len(req.Buffer) >= 1024 {
				copy(req.Buffer[:1024], DiskImages[drive][byteOff:byteOff+1024])
			}
			EndRequestForDev(MAJOR_NR_HD, 1)
		} else if req.Cmd == WRITE {
			if byteOff+1024 <= len(DiskImages[drive]) && len(req.Buffer) >= 1024 {
				copy(DiskImages[drive][byteOff:byteOff+1024], req.Buffer[:1024])
			}
			EndRequestForDev(MAJOR_NR_HD, 1)
		}
	} else {
		EndRequestForDev(MAJOR_NR_HD, 0)
	}
	// Process next request if any
	if BlkDev[MAJOR_NR_HD].CurrentRequest != nil {
		DoHdRequest()
	}
}

// hd.c: hd_init
func HdInit() {
	BlkDev[MAJOR_NR_HD].RequestFn = DoHdRequest
	log.Printf("hd: hd_init done, %d drives", NrHd)
}

// SetDiskImage: configure an in-memory disk for the Go port
func SetDiskImage(drive int, data []byte) {
	if drive >= 0 && drive < MAX_HD {
		DiskImages[drive] = data
		if drive >= NrHd { NrHd = drive + 1 }
		// Set default geometry
		sectors := len(data) / 512
		HdInfo[drive].Sect = 63
		HdInfo[drive].Head = 16
		if HdInfo[drive].Sect*HdInfo[drive].Head > 0 {
			HdInfo[drive].Cyl = sectors / (HdInfo[drive].Sect * HdInfo[drive].Head)
		}
		Hd[drive*5].StartSect = 0
		Hd[drive*5].NrSects = int32(sectors)
	}
}
