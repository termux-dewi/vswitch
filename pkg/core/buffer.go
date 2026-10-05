package core

import "sync"

const MaxFrameSize = 1518

var BufferPool = sync.Pool{New: func() any { return make([]byte, MaxFrameSize) }}

func GetBuffer(n int) []byte {
	if n <= MaxFrameSize {
		return BufferPool.Get().([]byte)[:n]
	}
	return make([]byte, n)
}
func PutBuffer(b []byte) {
	if cap(b) == MaxFrameSize {
		BufferPool.Put(b[:MaxFrameSize])
	}
}
