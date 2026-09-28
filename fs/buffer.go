package fs
import "sync"
const NR_HASH = 307
type bufferCache struct {
	mu    sync.RWMutex
	cache map[uint64]*buffer_head
}
var bcache = &bufferCache{cache: make(map[uint64]*buffer_head)}
func hashKey(dev uint16, block uint64) uint64 { return uint64(dev)<<32 | block }
func getblk(dev int, block int) *buffer_head {
	key := hashKey(uint16(dev), uint64(block))
	bcache.mu.Lock()
	defer bcache.mu.Unlock()
	if bh, ok := bcache.cache[key]; ok {
		bh.b_count++
		return bh
	}
	bh := &buffer_head{
		b_data:    make([]byte, BLOCK_SIZE),
		b_blocknr: uint64(block),
		b_dev:     uint16(dev),
		b_count:   1,
	}
	bcache.cache[key] = bh
	return bh
}
func brelse(bh *buffer_head) {
	if bh == nil { return }
	bcache.mu.Lock()
	defer bcache.mu.Unlock()
	if bh.b_count > 0 { bh.b_count-- }
}
func bread(dev, block int) *buffer_head {
	bh := getblk(dev, block)
	if bh.b_uptodate != 0 { return bh }
	bh.b_uptodate = 1
	return bh
}
func sync_dev(dev int) {
	bcache.mu.RLock()
	defer bcache.mu.RUnlock()
	for _, bh := range bcache.cache {
		if bh.b_dev == uint16(dev) && bh.b_dirt != 0 {
			bh.b_dirt = 0
		}
	}
}
func buffer_init() {
	bcache.mu.Lock()
	defer bcache.mu.Unlock()
	bcache.cache = make(map[uint64]*buffer_head)
}
func Getblk(dev, block int) *buffer_head { return getblk(dev, block) }
func Brelse(bh *buffer_head)             { brelse(bh) }
func Bread(dev, block int) *buffer_head  { return bread(dev, block) }
func SyncDev(dev int)                    { sync_dev(dev) }
func BufferInit()                        { buffer_init() }
