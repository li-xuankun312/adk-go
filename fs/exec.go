// fs/exec.go — ported from linux-0.11/fs/exec.c
// (C) 1991 Linus Torvalds
//
// #!-checking implemented by tytso.
// Demand-loading implemented 01.12.91
package fs

import (
	"encoding/binary"
	"log"
	"strings"

	. "google.golang.org/adk/v2/include"
)

const (
	MAX_ARG_PAGES = 32
	ENOEXEC       = 8
	ENOMEM        = 12
	S_ISUID       = 04000
	S_ISGID       = 02000
	ZMAGIC        = 0x10B // a.out magic
	N_TXTOFF_VAL  = BLOCK_SIZE
)

// Exec header — matches a.out struct exec
type ExecHeader struct {
	AMagic  uint32
	AText   uint32
	AData   uint32
	ABss    uint32
	ASyms   uint32
	AEntry  uint32
	ATrsize uint32
	ADrsize uint32
}

// Callbacks for page/process ops
var (
	freePageTablesFn  func(uint32, uint32)
	putPageFn         func(uint32, uint32)
	sysExitFn         func(int)
	sysCloseFn        func(int) int
)

func SetFreePageTables(fn func(uint32, uint32)) { freePageTablesFn = fn }
func SetPutPage(fn func(uint32, uint32))        { putPageFn = fn }
func SetSysExit(fn func(int))                   { sysExitFn = fn }
func SetSysClose(fn func(int) int)              { sysCloseFn = fn }

// readExecHeader from buffer
func readExecHeader(data []byte) ExecHeader {
	var ex ExecHeader
	if len(data) < 32 { return ex }
	ex.AMagic = binary.LittleEndian.Uint32(data[0:])
	ex.AText = binary.LittleEndian.Uint32(data[4:])
	ex.AData = binary.LittleEndian.Uint32(data[8:])
	ex.ABss = binary.LittleEndian.Uint32(data[12:])
	ex.ASyms = binary.LittleEndian.Uint32(data[16:])
	ex.AEntry = binary.LittleEndian.Uint32(data[20:])
	ex.ATrsize = binary.LittleEndian.Uint32(data[24:])
	ex.ADrsize = binary.LittleEndian.Uint32(data[28:])
	return ex
}

// exec.c lines 46-70: create_tables (simplified for Go)
// In Go, we store args/envs directly in the task's memory area.
func createTables(args []string, envs []string) (uint32, []string, []string) {
	// Return a simulated stack pointer
	return PAGE_SIZE * MAX_ARG_PAGES - 4, args, envs
}

// exec.c lines 75-85: count — trivial in Go
func countArgs(argv []string) int { return len(argv) }

// exec.c lines 104-152: copy_strings — in Go, strings are already in memory
// This is a no-op in Go since we don't need segmented memory copying.
func copyStrings(argv []string, page []uint32, p uint32) uint32 {
	for _, s := range argv {
		slen := uint32(len(s) + 1) // include null terminator
		if p < slen { return 0 }
		p -= slen
	}
	return p
}

// exec.c lines 154-177: change_ldt (simplified)
func changeLdt(textSize uint32, page []uint32) uint32 {
	// In Go, we don't manipulate LDT/GDT. We just set limits.
	dataLimit := uint32(0x4000000)
	return dataLimit
}

// exec.c lines 182-353: do_execve
func DoExecve(filename string, argv []string, envp []string) int {
	var page [MAX_ARG_PAGES]uint32
	argc := countArgs(argv)
	envc := countArgs(envp)
	p := uint32(PAGE_SIZE*MAX_ARG_PAGES - 4)

	inode := Namei(filename)
	if inode == nil { return -ENOENT }

	shBang := false

restart_interp:
	if !S_ISREG(inode.IMode) {
		Iput(inode); goto exec_error1
	}
	// Permission check
	i := int(inode.IMode)
	eUid := Current.Euid
	eGid := Current.Egid
	if i&S_ISUID != 0 { eUid = inode.IUid }
	if i&S_ISGID != 0 { eGid = uint16(inode.IGid) }
	if Current.Euid == inode.IUid { i >>= 6 } else if uint8(Current.Egid) == inode.IGid { i >>= 3 }
	if i&1 == 0 && !((inode.IMode&0111) != 0 && suser()) {
		Iput(inode); goto exec_error1
	}

	bh := Bread(int(inode.IDev), int(inode.IZone[0]))
	if bh == nil { Iput(inode); goto exec_error1 }
	ex := readExecHeader(bh.BData)

	// #! interpreter check
	if len(bh.BData) >= 2 && bh.BData[0] == '#' && bh.BData[1] == '!' && !shBang {
		// Parse interpreter line
		end := strings.IndexByte(string(bh.BData[2:]), '\n')
		if end < 0 { end = 1022 }
		if end > 1022 { end = 1022 }
		line := strings.TrimSpace(string(bh.BData[2 : 2+end]))
		Brelse(bh)
		Iput(inode)
		if line == "" { goto exec_error1 }
		parts := strings.Fields(line)
		interp := parts[0]
		if !shBang {
			shBang = true
			p = copyStrings(envp, page[:], p)
			if argc > 1 { p = copyStrings(argv[1:], page[:], p) }
		}
		// Rebuild argv: [interpreter_name, optional_arg, filename, original_args...]
		newArgv := []string{interp}
		if len(parts) > 1 { newArgv = append(newArgv, parts[1:]...); }
		newArgv = append(newArgv, filename)
		if argc > 1 { newArgv = append(newArgv, argv[1:]...) }
		argv = newArgv
		argc = len(argv)

		inode = Namei(interp)
		if inode == nil { goto exec_error1 }
		goto restart_interp
	}

	Brelse(bh)

	// Validate a.out header
	if ex.AMagic != ZMAGIC || ex.ATrsize != 0 || ex.ADrsize != 0 ||
		ex.AText+ex.AData+ex.ABss > 0x3000000 ||
		inode.ISize < ex.AText+ex.AData+ex.ASyms+N_TXTOFF_VAL {
		Iput(inode)
		goto exec_error1
	}

	if !shBang {
		p = copyStrings(envp, page[:], p)
		p = copyStrings(argv, page[:], p)
		if p == 0 { Iput(inode); goto exec_error1 }
	}

	// Point of no return
	if Current.Executable != nil {
		Iput(Current.Executable)
	}
	Current.Executable = inode
	for j := 0; j < 32; j++ {
		Current.Sigaction[j].SaHandler = 0
	}
	for j := 0; j < NR_OPEN; j++ {
		if (Current.CloseOnExec>>j)&1 != 0 {
			if sysCloseFn != nil { sysCloseFn(j) }
		}
	}
	Current.CloseOnExec = 0
	// free_page_tables — simplified
	Current.UsedMath = 0
	p += changeLdt(ex.AText, page[:]) - MAX_ARG_PAGES*PAGE_SIZE
	Current.Brk = int32(ex.ABss + ex.AData + ex.AText)
	Current.EndData = int32(ex.AData + ex.AText)
	Current.EndCode = int32(ex.AText)
	Current.StartStack = int32(p & 0xFFFFF000)
	Current.Euid = eUid
	Current.Egid = eGid
	return 0

exec_error1:
	for j := 0; j < MAX_ARG_PAGES; j++ {
		if page[j] != 0 && freePageFn != nil {
			freePageFn(page[j])
		}
	}
	log.Printf("fs: exec error for %s", filename)
	return -ENOEXEC
}
