// include/sched.go — ported from linux-0.11/include/linux/sched.h
// Line-for-line translation of task_struct and scheduling infrastructure.
package include

import "sync"

// sched.h lines 4-5
const (
	NR_TASKS = 64
	HZ       = 100
)

// sched.h lines 19-23: task states
const (
	TASK_RUNNING         = 0
	TASK_INTERRUPTIBLE   = 1
	TASK_UNINTERRUPTIBLE = 2
	TASK_ZOMBIE          = 3
	TASK_STOPPED         = 4
)

// sched.h line 40: typedef int (*fn_ptr)();
type FnPtr func()

// sched.h lines 42-51: struct i387_struct
// FPU state — kept for structural fidelity, unused in Go port.
type I387Struct struct {
	Cwd     int32      // long cwd
	Swd     int32      // long swd
	Twd     int32      // long twd
	Fip     int32      // long fip
	Fcs     int32      // long fcs
	Foo     int32      // long foo
	Fos     int32      // long fos
	StSpace [20]int32  // long st_space[20] — 8*10 bytes for each FP-reg = 80 bytes
}

// sched.h lines 53-78: struct tss_struct
// Task State Segment — x86 hardware context. In Go, goroutines replace
// hardware context switching. We keep the struct for data compatibility.
type TssStruct struct {
	BackLink int32     // long back_link (16 high bits zero)
	Esp0     int32     // long esp0
	Ss0      int32     // long ss0
	Esp1     int32     // long esp1
	Ss1      int32     // long ss1
	Esp2     int32     // long esp2
	Ss2      int32     // long ss2
	Cr3      int32     // long cr3
	Eip      int32     // long eip
	Eflags   int32     // long eflags
	Eax      int32     // long eax
	Ecx      int32     // long ecx
	Edx      int32     // long edx
	Ebx      int32     // long ebx
	Esp      int32     // long esp
	Ebp      int32     // long ebp
	Esi      int32     // long esi
	Edi      int32     // long edi
	Es       int32     // long es
	Cs       int32     // long cs
	Ss       int32     // long ss
	Ds       int32     // long ds
	Fs       int32     // long fs
	Gs       int32     // long gs
	Ldt      int32     // long ldt
	TraceBitmap int32  // long trace_bitmap
	I387     I387Struct // struct i387_struct i387
}

// sched.h lines 80-109: struct task_struct
// This is THE central data structure of the kernel.
// Original C struct preserved field-by-field.
type TaskStruct struct {
	// --- hardcoded section (sched.h lines 81-87) ---
	State    int32             // long state — -1 unrunnable, 0 runnable, >0 stopped
	Counter  int32             // long counter — time slice remaining
	Priority int32             // long priority — scheduling priority
	Signal   int32             // long signal — bitmap of pending signals
	Sigaction [32]Sigaction    // struct sigaction sigaction[32]
	Blocked  int32             // long blocked — bitmap of masked signals

	// --- various fields (sched.h lines 88-96) ---
	ExitCode   int32           // int exit_code
	StartCode  uint32          // unsigned long start_code
	EndCode    uint32          // unsigned long end_code
	EndData    uint32          // unsigned long end_data
	Brk        uint32          // unsigned long brk
	StartStack uint32          // unsigned long start_stack
	Pid        int32           // long pid
	Father     int32           // long father
	Pgrp       int32           // long pgrp
	Session    int32           // long session
	Leader     int32           // long leader
	Uid        uint16          // unsigned short uid
	Euid       uint16          // unsigned short euid
	Suid       uint16          // unsigned short suid
	Gid        uint16          // unsigned short gid
	Egid       uint16          // unsigned short egid
	Sgid       uint16          // unsigned short sgid
	Alarm      int32           // long alarm
	Utime      int32           // long utime
	Stime      int32           // long stime
	Cutime     int32           // long cutime
	Cstime     int32           // long cstime
	StartTime  int32           // long start_time
	UsedMath   uint16          // unsigned short used_math

	// --- file system info (sched.h lines 97-104) ---
	Tty        int32           // int tty — -1 if no tty
	Umask      uint16          // unsigned short umask
	Pwd        *MInode         // struct m_inode * pwd
	Root       *MInode         // struct m_inode * root
	Executable *MInode         // struct m_inode * executable
	CloseOnExec uint32         // unsigned long close_on_exec
	Filp       [NR_OPEN]*File  // struct file * filp[NR_OPEN]

	// --- segment descriptors (sched.h lines 105-108) ---
	Ldt [3]DescStruct          // struct desc_struct ldt[3]
	Tss TssStruct              // struct tss_struct tss

	// --- Go port additions for concurrency ---
	// In Linux 0.11, context switching is done via hardware TSS.
	// In Go, each task runs as a goroutine. These fields replace
	// the ljmp-based switch_to mechanism.
	Mu   sync.Mutex            // protects concurrent field access
}

// sched.h lines 138-142: global variables
var (
	Task             [NR_TASKS]*TaskStruct // struct task_struct *task[NR_TASKS]
	LastTaskUsedMath *TaskStruct           // struct task_struct *last_task_used_math
	Current          *TaskStruct           // struct task_struct *current
	Jiffies          int32                 // long volatile jiffies
	StartupTime      int32                 // long startup_time
)

// sched.h line 144
func CURRENT_TIME() int32 {
	return StartupTime + Jiffies/HZ
}

// sched.h lines 7-8
func FIRST_TASK() *TaskStruct { return Task[0] }
func LAST_TASK() *TaskStruct  { return Task[NR_TASKS-1] }

// sched.h lines 155-158: TSS/LDT entry calculations
const (
	FIRST_TSS_ENTRY = 4
	FIRST_LDT_ENTRY = FIRST_TSS_ENTRY + 1
)

func TSS(n int) uint32 { return uint32(n)<<4 + FIRST_TSS_ENTRY<<3 }
func LDT(n int) uint32 { return uint32(n)<<4 + FIRST_LDT_ENTRY<<3 }

// sched.h line 188
func PAGE_ALIGN(n uint32) uint32 { return (n + 0xfff) &^ 0xfff }

// _set_base, _set_limit, _get_base, get_limit: these manipulate x86 segment
// descriptors. In the Go port, we implement them as field accessors on
// DescStruct rather than inline assembly.

// set_base: store a 32-bit linear base address into a segment descriptor
func SetBase(desc *DescStruct, base uint32) {
	desc.A = (desc.A & 0x0000FFFF) | ((base & 0x0000FFFF) << 16)
	desc.B = (desc.B & 0x00FFFF00) | ((base >> 16) & 0xFF) | (base & 0xFF000000)
}

// get_base: extract the 32-bit linear base address from a segment descriptor
func GetBase(desc DescStruct) uint32 {
	return (desc.A >> 16) | ((desc.B & 0xFF) << 16) | (desc.B & 0xFF000000)
}

// set_limit: store a 20-bit page-granularity limit (limit in pages, shifted >>12)
func SetLimit(desc *DescStruct, limit uint32) {
	limit = (limit - 1) >> 12
	desc.A = (desc.A & 0xFFFF0000) | (limit & 0x0000FFFF)
	desc.B = (desc.B & 0xFFF0FFFF) | (limit & 0x000F0000)
}

// get_limit: extract limit (returns byte count after lsl simulation)
func GetLimit(desc DescStruct) uint32 {
	limit := (desc.A & 0x0000FFFF) | (desc.B & 0x000F0000)
	return (limit << 12) | 0xFFF // page granularity → byte limit
}
