package fs

import (
	"encoding/binary"
	"log"
	"strings"

	. "google.golang.org/adk/v2/include"
)

const (
	MAX_ARG_PAGES = 32
	S_ISUID       = 04000
	S_ISGID       = 02000
	ZMAGIC        = 0x10B
	N_TXTOFF_VAL  = BLOCK_SIZE
)

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

func createTables(args []string, envs []string) (uint32, []string, []string) {
	return PAGE_SIZE * MAX_ARG_PAGES - 4, args, envs
}

func countArgs(argv []string) int { return len(argv) }

func copyStrings(argv []string, page []uint32, p uint32) uint32 {
	for _, s := range argv {
		slen := uint32(len(s) + 1)
		if p < slen { return 0 }
		p -= slen
	}
	return p
}

func changeLdt(textSize uint32, page []uint32) uint32 {
	dataLimit := uint32(0x4000000)
	return dataLimit
}

func DoExecve(filename string, argv []string, envp []string) int {
	var page [MAX_ARG_PAGES]uint32
	var bh *BufferHead
	var ex ExecHeader
	var eUid uint16
	var eGid uint16
	argc := countArgs(argv)
	envc := countArgs(envp)
	_ = envc
	p := uint32(PAGE_SIZE*MAX_ARG_PAGES - 4)

	inode := Namei(filename)
	if inode == nil { return -ENOENT }

	shBang := false

var i int
restart_interp:
	if !S_ISREG(inode.IMode) {
		Iput(inode); goto exec_error1
	}
	i = int(inode.IMode)
	eUid = Current.Euid
	eGid = Current.Egid
	if i&S_ISUID != 0 { eUid = inode.IUid }
	if i&S_ISGID != 0 { eGid = uint16(inode.IGid) }
	if Current.Euid == inode.IUid { i >>= 6 } else if uint8(Current.Egid) == inode.IGid { i >>= 3 }
	if i&1 == 0 && !((inode.IMode&0111) != 0 && suser()) {
		Iput(inode); goto exec_error1
	}

	bh = Bread(int(inode.IDev), int(inode.IZone[0]))
	if bh == nil { Iput(inode); goto exec_error1 }
	ex = readExecHeader(bh.BData)

	if len(bh.BData) >= 2 && bh.BData[0] == '#' && bh.BData[1] == '!' && !shBang {
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

	if Current.Executable != nil {
		Iput(Current.Executable)
	}
	Current.Executable = inode
	for j := 0; j < 32; j++ {
		Current.Sigaction[j].SaHandler = nil
	}
	for j := 0; j < NR_OPEN; j++ {
		if (Current.CloseOnExec>>j)&1 != 0 {
			if sysCloseFn != nil { sysCloseFn(j) }
		}
	}
	Current.CloseOnExec = 0
	Current.UsedMath = 0
	p += changeLdt(ex.AText, page[:]) - MAX_ARG_PAGES*PAGE_SIZE
	Current.Brk = uint32(ex.ABss + ex.AData + ex.AText)
	Current.EndData = uint32(ex.AData + ex.AText)
	Current.EndCode = uint32(ex.AText)
	Current.StartStack = uint32(p & 0xFFFFF000)
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
