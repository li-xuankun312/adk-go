package blk_drv
import (
	"fmt"
	"sync"
)
const NR_REQUEST = 32
const (
	IN_ORDER = iota
	SORTED
)
type request struct {
	dev     int
	cmd     int
	sector  uint64
	buffer  []byte
	waiting chan struct{}
	next    *request
}
type blk_dev_struct struct {
	request_fn func(*request)
	current    *request
	mu         sync.Mutex
}
var blk_dev [7]blk_dev_struct
var request_pool [NR_REQUEST]request
func ll_rw_block(rw int, dev int, block uint64, buf []byte) error {
	if dev < 0 || dev >= 7 { return fmt.Errorf("ll_rw_block: bad dev %d", dev) }
	bd := &blk_dev[dev]
	bd.mu.Lock()
	defer bd.mu.Unlock()
	req := &request{dev: dev, cmd: rw, sector: block * 2, buffer: buf, waiting: make(chan struct{})}
	if bd.request_fn != nil { bd.request_fn(req) }
	return nil
}
func blk_dev_init() {
	for i := 0; i < NR_REQUEST; i++ {
		request_pool[i] = request{dev: -1}
	}
}
func LlRwBlock(rw, dev int, block uint64, buf []byte) error { return ll_rw_block(rw, dev, block, buf) }
func Init()                                                  { blk_dev_init() }
