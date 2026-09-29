// lib/malloc.go — ported from linux-0.11/lib/malloc.c
// (C) 1991 Linus Torvalds
//
// Bucket-based kernel memory allocator. In the Go port, we use Go's
// built-in allocator (make/new) but provide the API for structural fidelity.
package lib

import "sync"

// malloc.c constants
const (
	MIN_BUCKET_SIZE = 16
	MAX_BUCKET_SIZE = 4096
)

// malloc.c: struct bucket_desc
type BucketDesc struct {
	Next       *BucketDesc
	Page       []byte
	RefCnt     int
	BucketSize int
}

// malloc.c: struct _bucket_dir
type BucketDir struct {
	Size  int
	Chain *BucketDesc
}

// malloc.c: bucket directory — the sizes available
var bucketDir = []BucketDir{
	{16, nil}, {32, nil}, {64, nil}, {128, nil},
	{256, nil}, {512, nil}, {1024, nil}, {2048, nil}, {4096, nil},
}

var mallocMu sync.Mutex

// malloc.c lines 90-160: kernel_malloc
func KernelMalloc(size int) []byte {
	mallocMu.Lock()
	defer mallocMu.Unlock()
	if size <= 0 { return nil }
	// Find best-fit bucket
	for _, dir := range bucketDir {
		if dir.Size >= size {
			return make([]byte, dir.Size)
		}
	}
	// Larger than max bucket — allocate directly
	return make([]byte, size)
}

// malloc.c lines 162-232: kernel_free
// In Go, freed memory is handled by the garbage collector.
func KernelFree(obj []byte) {
	// No-op in Go — GC handles deallocation
}
