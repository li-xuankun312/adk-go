// init/main.go — ported from linux-0.11/init/main.c
// (C) 1991 Linus Torvalds
//
// This is the kernel initialization and first process (init) code.
// In the original, this is the entry point after boot assembly.
// In Go, this is called from the claudeweb main adapter.
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

// main.c globals
var (
	memoryEnd        int32 = 16 * 1024 * 1024 // 16MB default
	bufferMemoryEnd  int32 = 4 * 1024 * 1024
	mainMemoryStart  int32 = 4 * 1024 * 1024
	StartupTime      int64
)

// main.c lines 78-98: time_init
func timeInit() {
	StartupTime = time.Now().Unix()
	kernel.SetStartupTime(StartupTime)
}

// main.c lines 106-152: main — kernel initialization sequence
func KernelMain() {
	log.Println("linux-0.11-go: Booting...")

	// Set ROOT_DEV (normally from BIOS at 0x901FC)
	ROOT_DEV = 0x301 // first hard disk, first partition

	// Memory layout
	// In the original: memory_end from EXT_MEM_K
	// In Go: configurable, default 16MB
	if memoryEnd > 16*1024*1024 { memoryEnd = 16 * 1024 * 1024 }
	if memoryEnd > 12*1024*1024 {
		bufferMemoryEnd = 4 * 1024 * 1024
	} else if memoryEnd > 6*1024*1024 {
		bufferMemoryEnd = 2 * 1024 * 1024
	} else {
		bufferMemoryEnd = 1 * 1024 * 1024
	}
	mainMemoryStart = bufferMemoryEnd

	// Initialize subsystems
	mm.MemInit(uint32(mainMemoryStart), uint32(memoryEnd))       // mem_init
	kernel.TrapInit()                                              // trap_init
	blk_drv.BlkDevInit()                                           // blk_dev_init
	chr_drv.ChrDevInit()                                           // chr_dev_init
	chr_drv.TtyInit()                                              // tty_init
	timeInit()                                                      // time_init
	kernel.SchedInit()                                             // sched_init
	fs.BufferInit(int(bufferMemoryEnd))                            // buffer_init
	blk_drv.HdInit()                                               // hd_init
	// floppy_init() — omitted

	// Wire cross-package function variables
	wireCallbacks()

	log.Println("linux-0.11-go: Initialization complete.")

	// In the original: move_to_user_mode(), fork(), init()
	// In Go, we call init() directly as a goroutine
	go initProcess()

	// Task 0 idle loop
	log.Println("linux-0.11-go: Entering idle loop (task 0)")
	for {
		kernel.Schedule()
		time.Sleep(10 * time.Millisecond) // prevent busy-spin
	}
}

// Wire cross-package callbacks to avoid circular imports
func wireCallbacks() {
	// kernel → mm
	kernel.SetWriteVerify(mm.WriteVerify)

	// fs → kernel
	fs.SetSleepOn(kernel.SleepOn)
	fs.SetWakeUp(kernel.WakeUp)

	// blk_drv → kernel
	blk_drv.SetSleepOn(kernel.SleepOn)
	blk_drv.SetWakeUp(kernel.WakeUp)

	// fs → blk_drv
	fs.SetLlRwBlock(blk_drv.LlRwBlock)

	// blk_drv → fs
	blk_drv.SetBread(fs.Bread)
	blk_drv.SetBrelse(fs.Brelse)

	// fs → mm
	fs.SetFreePage(mm.FreePage)

	// chr_drv → kernel
	chr_drv.SetInterruptibleSleepOn(kernel.InterruptibleSleepOn)
	chr_drv.SetWakeUp(kernel.WakeUp)
	chr_drv.SetSchedule(kernel.Schedule)

	// fs/char_dev → chr_drv
	fs.SetTtyRead(chr_drv.TtyRead)
	fs.SetTtyWrite(chr_drv.TtyWrite)

	// fs/ioctl → chr_drv
	fs.SetTtyIoctl(chr_drv.TtyIoctl)
}

// main.c lines 171-211: init — process 1
func initProcess() {
	log.Println("linux-0.11-go: init process started")

	// sys_setup — read partition tables
	blk_drv.SysSetup()

	// Open /dev/tty0 as stdin/stdout/stderr
	// In the original: open("/dev/tty0", O_RDWR, 0); dup(0); dup(0)
	log.Printf("linux-0.11-go: %d buffers = %d bytes buffer space",
		fs.NR_BUFFERS, fs.NR_BUFFERS*BLOCK_SIZE)

	// Mount root filesystem
	fs.MountRoot()

	log.Println("linux-0.11-go: init complete — root filesystem mounted")
}
