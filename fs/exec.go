// fs/exec.go — ported from linux-0.11/fs/exec.c
// (C) 1991 Linus Torvalds
//
// Demand-loading implemented 01.12.91 - no need to read anything but
// the header into memory. The inode of the executable is put into
// "current->executable", and page faults do the actual loading.
package fs

import (
	"encoding/binary"
	"log"
	"strings"

	. "google.golang.org/adk/v2/include"
)

const MAX_ARG_PAGES = 32
const S_ISUID = 04000
const S_ISGID = 02000
const ENOMEM  = 12
const ENOEXEC = 8

// a.out header (struct exec) — simplified for Go port
type ExecHeader struct {
	AMagic   uint32
	AText    uint32
	AData    uint32
	ABss     uint32
	ASyms    uint32
	AEntry   uint32
	ATrsize  uint32
	ADrsize  uint32
}

const ZMAGIC = 0x10B // a.out magic number
const N_TXTOFF_VAL = BLOCK_SIZE

func readExecHeader(data []byte) ExecHeader {
	if len(data) < 32 { return ExecHeader{} }
	return ExecHeader{
		AMagic:  binary.LittleEndian.Uint32(data[0:]),
		AText:   binary.LittleEndian.Uint32(data[4:]),
		AData:   binary.LittleEndian.Uint32(data[8:]),
		ABss:    binary.LittleEndian.Uint32(data[12:]),
		ASyms:   binary.LittleEndian.Uint32(data[16:]),
		AEntry:  binary.LittleEndian.Uint32(data[20:]),
		ATrsize: binary.LittleEndian.Uint32(data[24:]),
		ADrsize: binary.LittleEndian.Uint32(data[28:]),
	}
}

// Callbacks
var (
	sysExitFn         func(int)
	freePageTablesFn  func(uint32, uint32)
)

func SetSysExit(fn func(int))               { sysExitFn = fn }
func SetFreePageTables(fn func(uint32, uint32)) { freePageTablesFn = fn }

// exec.c lines 46-70: create_tables — simplified
// In Go, the argument/environment handling is very different since we don't have
// user-mode segmentation. We track args/env as Go slices.
func createTables(args []string, envs []string) (int, int) {
	return len(args), len(envs)
}

// exec.c lines 75-85: count
func countStrings(argv []string) int { return len(argv) }

// exec.c lines 182-353: do_execve — simplified for Go port
func DoExecve(filename string, argv []string, envp []string) int {
	inode := Namei(filename)
	if inode == nil { return -ENOENT }
	argc := countStrings(argv)
	envc := countStrings(envp)
	_ = envc

	if !S_ISREG(inode.IMode) { Iput(inode); return -EACCES }

	// Permission check
	i := int(inode.IMode)
	euid := int(Current.Euid)
	egid := int(Current.Egid)
	if i&S_ISUID != 0 { euid = int(inode.IUid) }
	if i&S_ISGID != 0 { egid = int(inode.IGid) }
	if Current.Euid == inode.IUid { i >>= 6 } else if uint8(Current.Egid) == inode.IGid { i >>= 3 }
	if i&1 == 0 && !((inode.IMode&0111) != 0 && suser()) {
		Iput(inode); return -ENOEXEC
	}

	// Read first block for header
	if inode.IZone[0] == 0 { Iput(inode); return -EACCES }
	bh := Bread(int(inode.IDev), int(inode.IZone[0]))
	if bh == nil { Iput(inode); return -EACCES }

	// Check for #! scripts
	if len(bh.BData) >= 2 && bh.BData[0] == '#' && bh.BData[1] == '!' {
		// Parse interpreter
		line := string(bh.BData[2:])
		Brelse(bh)
		if nl := strings.IndexByte(line, '\n'); nl >= 0 { line = line[:nl] }
		line = strings.TrimSpace(line)
		if line == "" { Iput(inode); return -ENOEXEC }
		parts := strings.Fields(line)
		interp := parts[0]
		Iput(inode)
		// Restart with interpreter
		newArgv := make([]string, 0, len(parts)+argc)
		newArgv = append(newArgv, parts...)
		newArgv = append(newArgv, filename)
		if argc > 1 { newArgv = append(newArgv, argv[1:]...) }
		return DoExecve(interp, newArgv, envp)
	}

	// Read a.out header
	ex := readExecHeader(bh.BData)
	Brelse(bh)

	if ex.AMagic != ZMAGIC || ex.ATrsize != 0 || ex.ADrsize != 0 ||
		ex.AText+ex.AData+ex.ABss > 0x3000000 {
		Iput(inode); return -ENOEXEC
	}

	// Point of no return
	if Current.Executable != nil { Iput(Current.Executable) }
	Current.Executable = inode
	for j := 0; j < 32; j++ { Current.Sigaction[j].SaHandler = 0 }
	for j := 0; j < NR_OPEN; j++ {
		if (Current.CloseOnExec>>j)&1 != 0 { SysCloseFS(j) }
	}
	Current.CloseOnExec = 0

	// Free old page tables — stubbed
	if freePageTablesFn != nil { freePageTablesFn(0, 0) }

	Current.Brk = ex.ABss + ex.AData + ex.AText
	Current.EndData = ex.AData + ex.AText
	Current.EndCode = ex.AText
	Current.StartStack = uint32(MAX_ARG_PAGES*PAGE_SIZE) & 0xFFFFF000
	Current.Euid = uint16(euid)
	Current.Egid = uint16(egid)

	log.Printf("fs: execve: loaded %s, entry=0x%x, text=%d, data=%d",
		filename, ex.AEntry, ex.AText, ex.AData)
	return 0
}
