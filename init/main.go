package init011

import (
	"log"
	"time"

	"google.golang.org/adk/v2/blk_drv"
	"google.golang.org/adk/v2/chr_drv"
	"google.golang.org/adk/v2/fs"
	. "google.golang.org/adk/v2/include"
	"google.golang.org/adk/v2/kernel"
	"google.golang.org/adk/v2/mm"
)


var (
	memoryEnd        int32 = 16 * 1024 * 1024
	bufferMemoryEnd  int32 = 4 * 1024 * 1024
	mainMemoryStart  int32 = 4 * 1024 * 1024
	startupTS        int64
)

func timeInit() {
	startupTS = time.Now().Unix()
	kernel.SetStartupTime(startupTS)
}

func KernelMain() {
	log.Println("linux-0.11-go: Booting...")

	ROOT_DEV = 0x301

	if memoryEnd > 16*1024*1024 { memoryEnd = 16 * 1024 * 1024 }
	if memoryEnd > 12*1024*1024 {
		bufferMemoryEnd = 4 * 1024 * 1024
	} else if memoryEnd > 6*1024*1024 {
		bufferMemoryEnd = 2 * 1024 * 1024
	} else {
		bufferMemoryEnd = 1 * 1024 * 1024
	}
	mainMemoryStart = bufferMemoryEnd

	mm.MemInit(uint32(mainMemoryStart), uint32(memoryEnd))
	kernel.TrapInit()
	blk_drv.BlkDevInit()
	chr_drv.ChrDevInit()
	chr_drv.TtyInit()
	timeInit()
	kernel.SchedInit()
	fs.BufferInit(bufferMemoryEnd)
	blk_drv.HdInit()

	WireAll()

	log.Println("linux-0.11-go: Initialization complete.")

	go initProcess()

	log.Println("linux-0.11-go: Entering idle loop (task 0)")
	for {
		kernel.Schedule()
		time.Sleep(10 * time.Millisecond)
	}
}



func initProcess() {
	log.Println("linux-0.11-go: init process started")

	blk_drv.SysSetup()

	log.Printf("linux-0.11-go: %d buffers = %d bytes buffer space",
		fs.NrBuffers, fs.NrBuffers*BLOCK_SIZE)

	fs.MountRoot()

	log.Println("linux-0.11-go: init complete — root filesystem mounted")
}
