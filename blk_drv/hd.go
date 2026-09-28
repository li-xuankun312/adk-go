package blk_drv
import (
	"io"
	"os"
	"sync"
)
const (
	MAX_HD     = 2
	SECTOR_SIZE = 512
)
type hd_struct struct {
	start_sect uint64
	nr_sects   uint64
}
type hd_info struct {
	head  int
	sect  int
	cyl   int
	file  *os.File
	mu    sync.Mutex
}
var hd [MAX_HD * 5]hd_struct
var hd_info_table [MAX_HD]hd_info
func hd_init() {}
func do_hd_request(req *request) {
	if req == nil { return }
	dev := req.dev
	if dev >= MAX_HD { return }
	info := &hd_info_table[dev]
	info.mu.Lock()
	defer info.mu.Unlock()
	if info.file == nil { return }
	offset := int64(req.sector) * SECTOR_SIZE
	switch req.cmd {
	case 0: // READ
		info.file.Seek(offset, io.SeekStart)
		info.file.Read(req.buffer)
	case 1: // WRITE
		info.file.Seek(offset, io.SeekStart)
		info.file.Write(req.buffer)
	}
	if req.waiting != nil { close(req.waiting) }
}
func MountDisk(dev int, path string) error {
	if dev >= MAX_HD { return nil }
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil { return err }
	hd_info_table[dev].file = f
	blk_dev[3].request_fn = do_hd_request
	return nil
}
func HdInit() { hd_init() }
