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

type HdInfoStruct struct {
	Head, Sect, Cyl, Wpcom, Lzone, Ctl int
}

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

var DiskImages [MAX_HD][]byte

func SysSetup() int {
	for i := 0; i < NrHd; i++ {
		Hd[i*5].StartSect = 0
		Hd[i*5].NrSects = int32(HdInfo[i].Head * HdInfo[i].Sect * HdInfo[i].Cyl)
	}
	for drive := 0; drive < NrHd; drive++ {
		bh := Bread_blk(int(0x300+drive*5), 0)
		if bh == nil {
			log.Printf("hd: unable to read partition table of drive %d", drive)
			continue
		}
		if len(bh.BData) >= 0x1FE {
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

func DoHdRequest() {
	req := BlkDev[MAJOR_NR_HD].CurrentRequest
	if req == nil { return }
	if int(MAJOR(uint32(req.Dev))) != MAJOR_NR_HD {
		log.Printf("hd: request list destroyed")
		return
	}
	dev := int(MINOR(uint32(req.Dev)))
	drive := dev / 5
	if drive >= NrHd || drive >= MAX_HD {
		EndRequestForDev(MAJOR_NR_HD, 0)
		return
	}
	blockNr := int(req.Sector)
	startSect := int(Hd[dev].StartSect)
	nrSects := int(Hd[dev].NrSects)
	if blockNr+2 > nrSects {
		EndRequestForDev(MAJOR_NR_HD, 0)
		return
	}
	sector := blockNr + startSect
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
	if BlkDev[MAJOR_NR_HD].CurrentRequest != nil {
		DoHdRequest()
	}
}

func HdInit() {
	BlkDev[MAJOR_NR_HD].RequestFn = DoHdRequest
	log.Printf("hd: hd_init done, %d drives", NrHd)
}

func SetDiskImage(drive int, data []byte) {
	if drive >= 0 && drive < MAX_HD {
		DiskImages[drive] = data
		if drive >= NrHd { NrHd = drive + 1 }
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
