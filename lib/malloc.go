package lib
import "sync"
type mallocHeader struct {
	size int
}
type bucket struct {
	mu   sync.Mutex
	size int
	free [][]byte
}
var buckets = [...]bucket{
	{size: 16}, {size: 32}, {size: 64}, {size: 128},
	{size: 256}, {size: 512}, {size: 1024}, {size: 2048}, {size: 4096},
}
func Malloc(size int) []byte {
	for i := range buckets {
		if buckets[i].size >= size {
			b := &buckets[i]
			b.mu.Lock()
			if len(b.free) > 0 {
				buf := b.free[len(b.free)-1]
				b.free = b.free[:len(b.free)-1]
				b.mu.Unlock()
				return buf
			}
			b.mu.Unlock()
			return make([]byte, b.size)
		}
	}
	return make([]byte, size)
}
func Free(buf []byte) {
	size := cap(buf)
	for i := range buckets {
		if buckets[i].size >= size {
			b := &buckets[i]
			b.mu.Lock()
			b.free = append(b.free, buf[:b.size])
			b.mu.Unlock()
			return
		}
	}
}
