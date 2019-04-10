package buffer

import (
	"fmt"
	"strings"
	"sync"
)

// Buff .
type Buff struct {
	Len  int
	Data [1 << 16]byte
}

func (b *Buff) String() string {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < b.Len; i++ {
		if i != 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("0x%02x", b.Data[i]))
	}
	sb.WriteString("]")
	return sb.String()
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
