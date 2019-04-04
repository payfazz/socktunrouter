package buffer

import (
	"sync"
)

// Buff .
type Buff struct {
	Len  int
	Data [1 << 16]byte
}

var pool = sync.Pool{
	New: func() interface{} { return &Buff{} },
}

// Get .
// Buff is strongly owned by caller,
// Put must be called to release the resource.
func Get() *Buff {
	b := pool.Get().(*Buff)
	b.Len = 0
	return b
}

// Put .
func Put(b *Buff) {
	pool.Put(b)
}
