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

	wireCallbacks()

	log.Println("linux-0.11-go: Initialization complete.")

	go initProcess()

	log.Println("linux-0.11-go: Entering idle loop (task 0)")
	for {
		kernel.Schedule()
		time.Sleep(10 * time.Millisecond)
	}
}

func wireCallbacks() {
	kernel.SetWriteVerify(mm.WriteVerify)

	fs.SetSleepOn(kernel.SleepOn)
	fs.SetWakeUp(kernel.WakeUp)

	blk_drv.SetSleepOn(kernel.SleepOn)
	blk_drv.SetWakeUp(kernel.WakeUp)

	fs.SetLlRwBlock(blk_drv.LlRwBlock)

	blk_drv.SetBread(fs.Bread)
	blk_drv.SetBrelse(fs.Brelse)

	fs.SetFreePage(mm.FreePage)

	chr_drv.SetInterruptibleSleepOn(kernel.InterruptibleSleepOn)
	chr_drv.SetWakeUp(kernel.WakeUp)
	chr_drv.SetSchedule(kernel.Schedule)

	fs.SetTtyRead(chr_drv.TtyRead)
	fs.SetTtyWrite(chr_drv.TtyWrite)

	fs.SetTtyIoctl(chr_drv.TtyIoctl)
}

func initProcess() {
	log.Println("linux-0.11-go: init process started")

	blk_drv.SysSetup()

	log.Printf("linux-0.11-go: %d buffers = %d bytes buffer space",
		fs.NrBuffers, fs.NrBuffers*BLOCK_SIZE)

	fs.MountRoot()

	log.Println("linux-0.11-go: init complete — root filesystem mounted")
}
