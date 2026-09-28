package mm
import (
	"fmt"
	"log"
	"sync"
)
const (
	LOW_MEM       = 0x100000
	PAGING_MEMORY = 15 * 1024 * 1024
	PAGING_PAGES  = PAGING_MEMORY >> 12
	PAGE_SIZE     = 4096
	USED          = 100
)
var (
	HIGH_MEMORY int64
	mem_map     [PAGING_PAGES]byte
	mu          sync.Mutex
)
func MAP_NR(addr int64) int64 { return (addr - LOW_MEM) >> 12 }
func mem_init(start_mem, end_mem int64) {
	mu.Lock()
	defer mu.Unlock()
	HIGH_MEMORY = end_mem
	for i := 0; i < PAGING_PAGES; i++ {
		mem_map[i] = USED
	}
	i := MAP_NR(start_mem)
	end_mem -= start_mem
	end_mem >>= 12
	for end_mem > 0 {
		end_mem--
		mem_map[i] = 0
		i++
	}
}
func get_free_page() int64 {
	mu.Lock()
	defer mu.Unlock()
	for i := PAGING_PAGES - 1; i >= 0; i-- {
		if mem_map[i] == 0 {
			mem_map[i] = 1
			return int64(i)<<12 + LOW_MEM
		}
	}
	return 0
}
func free_page(addr int64) {
	mu.Lock()
	defer mu.Unlock()
	if addr < LOW_MEM { return }
	if addr >= HIGH_MEMORY {
		log.Printf("mm: trying to free nonexistent page %x", addr)
		return
	}
	addr -= LOW_MEM
	addr >>= 12
	if mem_map[addr] > 0 { mem_map[addr]--; return }
	mem_map[addr] = 0
	log.Printf("mm: trying to free free page")
}
type PageTable struct {
	mu      sync.RWMutex
	entries map[int64]int64
}
func NewPageTable() *PageTable {
	return &PageTable{entries: make(map[int64]int64)}
}
func (pt *PageTable) Put(vaddr, paddr int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.entries[vaddr&0xFFFFF000] = paddr | 7
}
func (pt *PageTable) Get(vaddr int64) (int64, bool) {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	v, ok := pt.entries[vaddr&0xFFFFF000]
	if !ok || v&1 == 0 { return 0, false }
	return v & 0xFFFFF000, true
}
func (pt *PageTable) Free() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	for _, paddr := range pt.entries {
		if paddr&1 != 0 { free_page(paddr & 0xFFFFF000) }
	}
	pt.entries = make(map[int64]int64)
}
func (pt *PageTable) CopyFrom(src *PageTable) {
	src.mu.RLock()
	defer src.mu.RUnlock()
	pt.mu.Lock()
	defer pt.mu.Unlock()
	for vaddr, paddr := range src.entries {
		if paddr&1 == 0 { continue }
		physAddr := paddr & 0xFFFFF000
		paddr &= ^int64(2)
		src.entries[vaddr] = paddr
		pt.entries[vaddr] = paddr
		if physAddr >= LOW_MEM {
			mu.Lock()
			idx := MAP_NR(physAddr)
			if idx >= 0 && idx < PAGING_PAGES { mem_map[idx]++ }
			mu.Unlock()
		}
	}
}
func un_wp_page(pt *PageTable, vaddr int64) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	entry, ok := pt.entries[vaddr&0xFFFFF000]
	if !ok { return }
	oldPage := entry & 0xFFFFF000
	mu.Lock()
	idx := MAP_NR(oldPage)
	if oldPage >= LOW_MEM && idx >= 0 && idx < PAGING_PAGES && mem_map[idx] == 1 {
		pt.entries[vaddr&0xFFFFF000] = entry | 2
		mu.Unlock()
		return
	}
	mu.Unlock()
	newPage := get_free_page()
	if newPage == 0 { log.Printf("mm: oom in un_wp_page"); return }
	if oldPage >= LOW_MEM {
		mu.Lock()
		if idx >= 0 && idx < PAGING_PAGES { mem_map[idx]-- }
		mu.Unlock()
	}
	pt.entries[vaddr&0xFFFFF000] = newPage | 7
}
func do_wp_page(pt *PageTable, address int64) { un_wp_page(pt, address) }
func do_no_page(pt *PageTable, address int64) {
	address &= 0xFFFFF000
	page := get_free_page()
	if page == 0 { log.Printf("mm: oom in do_no_page"); return }
	pt.Put(address, page)
}
func write_verify(pt *PageTable, address int64) {
	pt.mu.RLock()
	entry, ok := pt.entries[address&0xFFFFF000]
	pt.mu.RUnlock()
	if !ok { return }
	if (entry & 3) == 1 { un_wp_page(pt, address) }
}
func calc_mem() {
	mu.Lock()
	defer mu.Unlock()
	free := 0
	for i := 0; i < PAGING_PAGES; i++ {
		if mem_map[i] == 0 { free++ }
	}
	fmt.Printf("%d pages free (of %d)\n", free, PAGING_PAGES)
}
func Init(start_mem, end_mem int64) { mem_init(start_mem, end_mem) }
func GetFreePage() int64            { return get_free_page() }
func FreePage(addr int64)           { free_page(addr) }
func CalcMem()                      { calc_mem() }
func DoWpPage(pt *PageTable, addr int64)  { do_wp_page(pt, addr) }
func DoNoPage(pt *PageTable, addr int64)  { do_no_page(pt, addr) }
func WriteVerify(pt *PageTable, addr int64) { write_verify(pt, addr) }
