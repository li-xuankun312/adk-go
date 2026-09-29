// kernel/fork.go — ported from linux-0.11/kernel/fork.c
// (C) 1991 Linus Torvalds
//
// 'fork.c' contains the help-routines for the 'fork' system call
// (see also system_call.s), and some misc functions ('verify_area').
// Fork is rather simple, once you get the hang of it, but the memory
// management can be a bitch. See 'mm/memory.go': 'CopyPageTables()'
package kernel

import (
	. "google.golang.org/adk/v2/include"
)

// fork.c line 23
var lastPid int32 = 0

// fork.c lines 25-38: verify_area
// Original walks pages calling write_verify to trigger COW.
// In Go port, we call into mm.WriteVerify for each page.
func VerifyArea(addr uint32, size int32) {
	// fork.c line 29-30
	start := addr
	size += int32(start & 0xfff)
	start &= 0xfffff000
	// fork.c line 32: start += get_base(current->ldt[2])
	start += GetBase(Current.Ldt[2])
	// fork.c lines 33-37
	for size > 0 {
		size -= 4096
		// write_verify is implemented in mm package
		// We call it through a function variable to avoid circular import
		if writeVerifyFn != nil {
			writeVerifyFn(start)
		}
		start += 4096
	}
}

// writeVerifyFn is set by mm package during init to avoid circular dependency
var writeVerifyFn func(address uint32)

// SetWriteVerify allows mm package to register its write_verify function
func SetWriteVerify(fn func(uint32)) {
	writeVerifyFn = fn
}

// fork.c lines 40-63: copy_mem
// Sets up LDT entries for the new process and copies page tables (COW).
func copyMem(nr int, p *TaskStruct) int {
	// fork.c lines 42-43
	var oldDataBase, newDataBase, dataLimit uint32
	var oldCodeBase, newCodeBase, codeLimit uint32

	// fork.c lines 45-48: get limits and bases from current LDT
	codeLimit = GetLimit(Current.Ldt[1]) // get_limit(0x0f)
	dataLimit = GetLimit(Current.Ldt[2]) // get_limit(0x17)
	oldCodeBase = GetBase(Current.Ldt[1])
	oldDataBase = GetBase(Current.Ldt[2])

	// fork.c lines 49-50
	if oldDataBase != oldCodeBase {
		Panic("We don't support separate I&D")
	}
	// fork.c lines 51-52
	if dataLimit < codeLimit {
		Panic("Bad data_limit")
	}

	// fork.c line 53: each process gets 64MB virtual address space
	newDataBase = uint32(nr) * 0x4000000
	newCodeBase = newDataBase
	p.StartCode = newCodeBase

	// fork.c lines 55-56: set segment bases in child's LDT
	SetBase(&p.Ldt[1], newCodeBase)
	SetBase(&p.Ldt[2], newDataBase)

	// fork.c lines 57-61: copy page tables (COW)
	if copyPageTablesFn != nil {
		if copyPageTablesFn(oldDataBase, newDataBase, int32(dataLimit)) != 0 {
			Printk("free_page_tables: from copy_mem\n")
			if freePageTablesFn != nil {
				freePageTablesFn(newDataBase, int32(dataLimit))
			}
			return -ENOMEM
		}
	}
	return 0
}

// Function variables for mm package to register (avoids circular import)
var copyPageTablesFn func(from, to uint32, size int32) int
var freePageTablesFn func(from uint32, size int32) int

func SetCopyPageTables(fn func(uint32, uint32, int32) int) { copyPageTablesFn = fn }
func SetFreePageTables(fn func(uint32, int32) int)         { freePageTablesFn = fn }

// Error codes from include/errno.go via dot import

// fork.c lines 70-138: copy_process
// Original takes register values from the stack frame.
// In Go, we don't have hardware registers — the caller passes relevant
// values. The signature is simplified but the logic is identical.
func CopyProcess(nr int, ebp, edi, esi, gs int32,
	ebx, ecx, edx int32,
	fs, es, ds int32,
	eip, cs, eflags, esp, ss int32) int32 {

	// fork.c line 79: p = (struct task_struct *) get_free_page()
	// In Go, we just allocate.
	p := &TaskStruct{}

	// fork.c line 82
	Task[nr] = p

	// fork.c line 86: *p = *current — copy parent's task_struct
	*p = *Current // NOTE: this is a shallow copy (same as C)

	// fork.c lines 87-96: override child-specific fields
	p.State = TASK_UNINTERRUPTIBLE
	p.Pid = lastPid
	p.Father = Current.Pid
	p.Counter = p.Priority
	p.Signal = 0
	p.Alarm = 0
	p.Leader = 0 // process leadership doesn't inherit
	p.Utime = 0
	p.Stime = 0
	p.Cutime = 0
	p.Cstime = 0
	p.StartTime = Jiffies

	// fork.c lines 97-117: set up child's TSS
	p.Tss.BackLink = 0
	p.Tss.Esp0 = PAGE_SIZE // would be PAGE_SIZE + (long)p in C
	p.Tss.Ss0 = 0x10
	p.Tss.Eip = eip
	p.Tss.Eflags = eflags
	p.Tss.Eax = 0 // fork returns 0 in child
	p.Tss.Ecx = ecx
	p.Tss.Edx = edx
	p.Tss.Ebx = ebx
	p.Tss.Esp = esp
	p.Tss.Ebp = ebp
	p.Tss.Esi = esi
	p.Tss.Edi = edi
	p.Tss.Es = es & 0xffff
	p.Tss.Cs = cs & 0xffff
	p.Tss.Ss = ss & 0xffff
	p.Tss.Ds = ds & 0xffff
	p.Tss.Fs = fs & 0xffff
	p.Tss.Gs = gs & 0xffff
	p.Tss.Ldt = int32(LDT(nr))
	p.Tss.TraceBitmap = int32(uint32(0x80000000))

	// fork.c lines 118-119: save FPU state if needed
	if LastTaskUsedMath == Current {
		// In Go, FPU state is not relevant
		// __asm__("clts ; fnsave %0":::"m" (p->tss.i387));
	}

	// fork.c lines 120-124: copy memory (page tables)
	if copyMem(nr, p) != 0 {
		Task[nr] = nil
		// In C, free_page((long)p); — in Go, GC handles it
		return -EAGAIN
	}

	// fork.c lines 125-127: increment file reference counts
	for i := 0; i < NR_OPEN; i++ {
		f := p.Filp[i]
		if f != nil {
			f.FCount++
		}
	}

	// fork.c lines 128-133: increment inode reference counts
	if Current.Pwd != nil {
		Current.Pwd.ICount++
	}
	if Current.Root != nil {
		Current.Root.ICount++
	}
	if Current.Executable != nil {
		Current.Executable.ICount++
	}

	// fork.c lines 134-135: set up GDT entries for child's TSS and LDT
	// In Go, we skip GDT manipulation.

	// fork.c line 136: do this last, just in case
	p.State = TASK_RUNNING

	// fork.c line 137
	return lastPid
}

// fork.c lines 140-152: find_empty_process
func FindEmptyProcess() int {
	// fork.c lines 144-147: find unused pid
repeat:
	lastPid++
	if lastPid < 0 {
		lastPid = 1
	}
	for i := 0; i < NR_TASKS; i++ {
		if Task[i] != nil && Task[i].Pid == lastPid {
			goto repeat
		}
	}
	// fork.c lines 148-151: find empty task slot
	for i := 1; i < NR_TASKS; i++ {
		if Task[i] == nil {
			return i
		}
	}
	return -EAGAIN
}
