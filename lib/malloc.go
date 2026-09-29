package lib

import "sync"

const (
	MIN_BUCKET_SIZE = 16
	MAX_BUCKET_SIZE = 4096
)

type BucketDesc struct {
	Next       *BucketDesc
	Page       []byte
	RefCnt     int
	BucketSize int
}

type BucketDir struct {
	Size  int
	Chain *BucketDesc
}

var bucketDir = []BucketDir{
	{16, nil}, {32, nil}, {64, nil}, {128, nil},
	{256, nil}, {512, nil}, {1024, nil}, {2048, nil}, {4096, nil},
}

var mallocMu sync.Mutex

func KernelMalloc(size int) []byte {
	mallocMu.Lock()
	defer mallocMu.Unlock()
	if size <= 0 { return nil }
	for _, dir := range bucketDir {
		if dir.Size >= size {
			return make([]byte, dir.Size)
		}
	}
	return make([]byte, size)
}

func KernelFree(obj []byte) {
}
