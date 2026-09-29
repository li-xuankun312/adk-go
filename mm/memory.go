// mm/memory.go — ported from linux-0.11/mm/memory.c
// (C) 1991 Linus Torvalds
//
// demand-loading started 01.12.91 - seems it is high on the list of
// things wanted, and it should be easy to implement. - Linus
//
// Ok, demand-loading was easy, shared pages a little bit tricker. Shared
// pages started 02.12.91, seems to work. - Linus.
package mm

import (
	"log"
	"sync"

	. "google.golang.org/adk/v2/include"
)

// memory.c lines 33-37: oom
func oom() {
	log.Printf("out of memory")
	// do_exit(SIGSEGV) — would need kernel import; handled via callback
	if doExitFn != nil {
		doExitFn(int32(SIGSEGV))
	}
}

var doExitFn func(int32)
func SetDoExit(fn func(int32)) { doExitFn = fn }

// memory.c line 39-40: invalidate() — flush TLB
// In Go, there's no TLB. This is a no-op.
func invalidate() {}

// memory.c lines 42-48: constants
const (
	LOW_MEM       = 0x100000       // 1MB
	PAGING_MEMORY = 15 * 1024 * 1024
	PAGING_PAGES  = PAGING_MEMORY >> 12
	USED          = 100
)

func MAP_NR(addr uint32) uint32 { return (addr - LOW_MEM) >> 12 }

// memory.c line 49-50: CODE_SPACE macro
// In Go, we access Current through the include package.
func CODE_SPACE(addr uint32) bool {
	return ((addr+4095)&^uint32(4095)) < Current.StartCode+Current.EndCode
}

// memory.c line 52
var HIGH_MEMORY uint32

// memory.c line 54-55: copy_page — copies 4096 bytes
func copyPage(from, to uint32) {
	// Original is "cld ; rep ; movsl" — copies 1024 longs (4096 bytes)
	// In Go, we copy between simulated physical memory regions.
	src := physMem[from : from+PAGE_SIZE]
	dst := physMem[to : to+PAGE_SIZE]
	copy(dst, src)
}

// memory.c line 57: static unsigned char mem_map[PAGING_PAGES] = {0,};
var memMap [PAGING_PAGES]uint8

// === SIMULATED PHYSICAL MEMORY ===
// Linux 0.11 operates on real physical RAM. In Go, we simulate this
// with a large byte array. Physical addresses are indices into this array.
var physMem []byte
var physMemMu sync.Mutex

// === SIMULATED PAGE DIRECTORY ===
// Linux 0.11 uses the hardware page directory at physical address 0.
// pg_dir[1024] in head.s. Each entry points to a page table.
// We simulate with a Go array. Format of entries:
//   bit 0: present
//   bit 1: writable
//   bit 2: user
//   bits 12-31: page table physical address
var pgDir [1024]uint32

// === SIMULATED PAGE TABLES ===
// Each page table has 1024 entries, each mapping a 4KB page.
// We store them inside physMem at the addresses referenced by pgDir.

// Helper: get a page table entry pointer (simulated)
func getPageTableEntry(pgTableAddr uint32, index int) *uint32 {
	offset := pgTableAddr + uint32(index)*4
	if int(offset+4) > len(physMem) {
		return nil
	}
	// We store uint32 values directly in a companion map for simplicity
	return &pageTableStore[offset>>2]
}

// pageTableStore simulates page table entries stored in physical memory.
// Key: physical address >> 2 (since each entry is 4 bytes).
var pageTableStore []uint32

// memory.c lines 63-83: get_free_page
// Original is inline assembly scanning mem_map backwards.
func GetFreePage() uint32 {
	physMemMu.Lock()
	defer physMemMu.Unlock()

	// Scan backwards through mem_map for a free page
	for i := PAGING_PAGES - 1; i >= 0; i-- {
		if memMap[i] == 0 {
			memMap[i] = 1
			addr := uint32(i)<<12 + LOW_MEM
			// Zero the page (stosl in original)
			if int(addr+PAGE_SIZE) <= len(physMem) {
				for j := uint32(0); j < PAGE_SIZE; j++ {
					physMem[addr+j] = 0
				}
			}
			return addr
		}
	}
	return 0 // no free pages
}

// memory.c lines 89-99: free_page
func FreePage(addr uint32) {
	if addr < LOW_MEM {
		return
	}
	if addr >= HIGH_MEMORY {
		log.Printf("mm: trying to free nonexistent page %x", addr)
		return
	}
	physMemMu.Lock()
	defer physMemMu.Unlock()
	addr -= LOW_MEM
	addr >>= 12
	if memMap[addr] > 0 {
		memMap[addr]--
		return
	}
	memMap[addr] = 0
	log.Printf("mm: trying to free free page")
}

// memory.c lines 105-131: free_page_tables
func FreePageTables(from uint32, size int32) int {
	if from&0x3fffff != 0 {
		log.Printf("mm: free_page_tables called with wrong alignment")
		return -1
	}
	if from == 0 {
		log.Printf("mm: Trying to free up swapper memory space")
		return -1
	}
	sz := uint32((int64(size) + 0x3fffff) >> 22)
	dirIdx := (from >> 22) // page directory index

	for ; sz > 0; sz-- {
		if pgDir[dirIdx]&1 == 0 {
			dirIdx++
			continue
		}
		pgTable := pgDir[dirIdx] & 0xfffff000
		for nr := uint32(0); nr < 1024; nr++ {
			entry := getPageTableEntry(pgTable, int(nr))
			if entry != nil && *entry&1 != 0 {
				FreePage(*entry & 0xfffff000)
			}
			if entry != nil {
				*entry = 0
			}
		}
		FreePage(pgDir[dirIdx] & 0xfffff000)
		pgDir[dirIdx] = 0
		dirIdx++
	}
	invalidate()
	return 0
}

// memory.c lines 150-189: copy_page_tables
// THE copy-on-write implementation for fork!
func CopyPageTables(from, to uint32, size int32) int {
	if (from&0x3fffff) != 0 || (to&0x3fffff) != 0 {
		log.Printf("mm: copy_page_tables called with wrong alignment")
		return -1
	}
	fromDir := from >> 22
	toDir := to >> 22
	sz := uint32((int64(size) + 0x3fffff) >> 22)

	for ; sz > 0; sz-- {
		if pgDir[toDir]&1 != 0 {
			log.Printf("mm: copy_page_tables: already exist")
			return -1
		}
		if pgDir[fromDir]&1 == 0 {
			fromDir++
			toDir++
			continue
		}
		fromPgTable := pgDir[fromDir] & 0xfffff000

		// Allocate a new page table for the destination
		toPgTableAddr := GetFreePage()
		if toPgTableAddr == 0 {
			return -1 // Out of memory
		}
		pgDir[toDir] = toPgTableAddr | 7 // present + writable + user

		// memory.c line 172: nr = (from==0)?0xA0:1024
		// For the first fork (copying kernel space), only copy 160 pages (640KB)
		nr := uint32(1024)
		if from == 0 {
			nr = 0xA0
		}

		for ; nr > 0; nr-- {
			idx := 1024 - int(nr) // index into page table
			fromEntry := getPageTableEntry(fromPgTable, idx)
			toEntry := getPageTableEntry(toPgTableAddr, idx)
			if fromEntry == nil || toEntry == nil {
				continue
			}

			thisPage := *fromEntry
			if thisPage&1 == 0 {
				continue
			}

			// memory.c line 177: clear writable bit (COW)
			thisPage &= ^uint32(2)
			*toEntry = thisPage

			// memory.c lines 179-184: if above LOW_MEM, also clear parent's
			// writable bit and increment reference count
			if thisPage > LOW_MEM {
				*fromEntry = thisPage
				physAddr := thisPage - LOW_MEM
				physAddr >>= 12
				physMemMu.Lock()
				if physAddr < PAGING_PAGES {
					memMap[physAddr]++
				}
				physMemMu.Unlock()
			}
		}
		fromDir++
		toDir++
	}
	invalidate()
	return 0
}

// memory.c lines 197-219: put_page
func PutPage(page, address uint32) uint32 {
	if page < LOW_MEM || page >= HIGH_MEMORY {
		log.Printf("mm: Trying to put page %x at %x", page, address)
	}
	physMemMu.Lock()
	mapIdx := (page - LOW_MEM) >> 12
	if mapIdx < PAGING_PAGES && memMap[mapIdx] != 1 {
		log.Printf("mm: mem_map disagrees with %x at %x", page, address)
	}
	physMemMu.Unlock()

	dirIdx := address >> 22
	if pgDir[dirIdx]&1 != 0 {
		// Page table exists
	} else {
		tmp := GetFreePage()
		if tmp == 0 {
			return 0
		}
		pgDir[dirIdx] = tmp | 7
	}
	pgTableAddr := pgDir[dirIdx] & 0xfffff000
	idx := (address >> 12) & 0x3ff
	entry := getPageTableEntry(pgTableAddr, int(idx))
	if entry != nil {
		*entry = page | 7
	}
	return page
}

// memory.c lines 221-238: un_wp_page
// Handles copy-on-write: if page has only one reference, just make it
// writable. Otherwise, copy the page and update the mapping.
func UnWpPage(tableEntry *uint32) {
	if tableEntry == nil {
		return
	}
	oldPage := *tableEntry & 0xfffff000

	physMemMu.Lock()
	mapIdx := MAP_NR(oldPage)
	if oldPage >= LOW_MEM && mapIdx < PAGING_PAGES && memMap[mapIdx] == 1 {
		*tableEntry |= 2 // make writable
		physMemMu.Unlock()
		invalidate()
		return
	}
	physMemMu.Unlock()

	newPage := GetFreePage()
	if newPage == 0 {
		oom()
		return
	}

	if oldPage >= LOW_MEM {
		physMemMu.Lock()
		mapIdx := MAP_NR(oldPage)
		if mapIdx < PAGING_PAGES {
			memMap[mapIdx]--
		}
		physMemMu.Unlock()
	}
	*tableEntry = newPage | 7
	invalidate()
	copyPage(oldPage, newPage)
}

// memory.c lines 247-258: do_wp_page
func DoWpPage(errorCode, address uint32) {
	// memory.c lines 255-257: find the page table entry
	dirIdx := address >> 22
	if pgDir[dirIdx]&1 == 0 {
		return
	}
	pgTableAddr := pgDir[dirIdx] & 0xfffff000
	idx := (address >> 12) & 0x3ff
	entry := getPageTableEntry(pgTableAddr, int(idx))
	if entry != nil {
		UnWpPage(entry)
	}
}

// memory.c lines 261-272: write_verify
func WriteVerify(address uint32) {
	dirIdx := address >> 22
	page := pgDir[dirIdx]
	if page&1 == 0 {
		return
	}
	page &= 0xfffff000
	idx := (address >> 12) & 0x3ff
	entry := getPageTableEntry(page, int(idx))
	if entry != nil && (*entry&3) == 1 { // non-writeable, present
		UnWpPage(entry)
	}
}

// memory.c lines 274-282: get_empty_page
func GetEmptyPage(address uint32) {
	tmp := GetFreePage()
	if tmp == 0 || PutPage(tmp, address) == 0 {
		FreePage(tmp) // 0 is ok — ignored
		oom()
	}
}

// memory.c lines 292-335: try_to_share
func tryToShare(address uint32, p *TaskStruct) int {
	fromPage := (address >> 22)
	toPage := fromPage

	fromPage += (p.StartCode >> 22)
	toPage += (Current.StartCode >> 22)

	// Is there a page-directory at from?
	from := pgDir[fromPage]
	if from&1 == 0 {
		return 0
	}
	from &= 0xfffff000
	fromIdx := (address >> 12) & 0x3ff
	fromEntry := getPageTableEntry(from, int(fromIdx))
	if fromEntry == nil {
		return 0
	}
	physAddr := *fromEntry

	// Is the page clean and present?
	if (physAddr & 0x41) != 0x01 {
		return 0
	}
	physAddr &= 0xfffff000
	if physAddr >= HIGH_MEMORY || physAddr < LOW_MEM {
		return 0
	}

	to := pgDir[toPage]
	if to&1 == 0 {
		newPt := GetFreePage()
		if newPt != 0 {
			pgDir[toPage] = newPt | 7
		} else {
			oom()
			return 0
		}
	}
	to = pgDir[toPage] & 0xfffff000
	toIdx := (address >> 12) & 0x3ff
	toEntry := getPageTableEntry(to, int(toIdx))
	if toEntry != nil && *toEntry&1 != 0 {
		log.Printf("mm: try_to_share: to_page already exists")
		return 0
	}

	// Share them: write-protect
	*fromEntry &= ^uint32(2)
	if toEntry != nil {
		*toEntry = *fromEntry
	}
	invalidate()

	physMemMu.Lock()
	physAddr -= LOW_MEM
	physAddr >>= 12
	if physAddr < PAGING_PAGES {
		memMap[physAddr]++
	}
	physMemMu.Unlock()
	return 1
}

// memory.c lines 345-364: share_page
func sharePage(address uint32) int {
	if Current.Executable == nil {
		return 0
	}
	if Current.Executable.ICount < 2 {
		return 0
	}
	for i := NR_TASKS - 1; i > 0; i-- {
		if Task[i] == nil {
			continue
		}
		if Current == Task[i] {
			continue
		}
		if Task[i].Executable != Current.Executable {
			continue
		}
		if tryToShare(address, Task[i]) != 0 {
			return 1
		}
	}
	return 0
}

// memory.c lines 366-398: do_no_page
// Demand paging: handle page fault for non-present page.
func DoNoPage(errorCode, address uint32) {
	address &= 0xfffff000
	tmp := address - Current.StartCode

	if Current.Executable == nil || tmp >= Current.EndData {
		GetEmptyPage(address)
		return
	}
	if sharePage(tmp) != 0 {
		return
	}

	page := GetFreePage()
	if page == 0 {
		oom()
		return
	}

	// memory.c lines 384-387: read from executable
	// block = 1 + tmp/BLOCK_SIZE
	// In Go, we'd need to call bmap and bread_page from the fs package.
	// For now, we just zero the page (no executable loading yet).
	// TODO: implement bread_page via fs callback

	// memory.c lines 388-393: zero out beyond end_data
	i := int32(tmp) + 4096 - int32(Current.EndData)
	tmpAddr := page + 4096
	for i > 0 {
		i--
		tmpAddr--
		if int(tmpAddr) < len(physMem) {
			physMem[tmpAddr] = 0
		}
	}

	if PutPage(page, address) != 0 {
		return
	}
	FreePage(page)
	oom()
}

// memory.c lines 400-412: mem_init
func MemInit(startMem, endMem uint32) {
	HIGH_MEMORY = endMem

	// Allocate simulated physical memory
	physMem = make([]byte, endMem)
	// Allocate page table entry store
	// Maximum possible entries: endMem/4 (each entry is 4 bytes)
	pageTableStore = make([]uint32, endMem/4)

	// Initialize mem_map: mark everything as USED initially
	for i := 0; i < PAGING_PAGES; i++ {
		memMap[i] = USED
	}
	// Mark pages from start_mem to end_mem as free
	i := MAP_NR(startMem)
	remain := (endMem - startMem) >> 12
	for remain > 0 {
		remain--
		if i < PAGING_PAGES {
			memMap[i] = 0
		}
		i++
	}

	log.Printf("mm: mem_init done, %d pages free", remain)
}

// memory.c lines 414-431: calc_mem
func CalcMem() {
	free := 0
	for i := 0; i < PAGING_PAGES; i++ {
		if memMap[i] == 0 {
			free++
		}
	}
	log.Printf("%d pages free (of %d)", free, PAGING_PAGES)

	// Walk page directory
	for i := 2; i < 1024; i++ {
		if pgDir[i]&1 != 0 {
			pgTable := pgDir[i] & 0xfffff000
			k := 0
			for j := 0; j < 1024; j++ {
				entry := getPageTableEntry(pgTable, j)
				if entry != nil && *entry&1 != 0 {
					k++
				}
			}
			log.Printf("Pg-dir[%d] uses %d pages", i, k)
		}
	}
}
