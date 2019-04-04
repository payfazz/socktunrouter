package util

import (
	"math/rand"
	"net"
	"time"

	"github.com/payfazz/socktunrouter/internal/buffer"
	"github.com/payfazz/socktunrouter/internal/done"
)

func init() {
	rand.Seed(time.Now().Unix())
}

// IPVersion .
func IPVersion(b *buffer.Buff) int {
	if b.Len == 0 {
		return -1
	}
	return int(b.Data[0] >> 4)
}

// IPv4Src .
func IPv4Src(b *buffer.Buff) net.IP {
	return net.IP(b.Data[12:16])
}

// IPv4Dst .
func IPv4Dst(b *buffer.Buff) net.IP {
	return net.IP(b.Data[16:20])
}

// IPv4Len .
func IPv4Len(b *buffer.Buff) int {
	return (int(b.Data[2])<<8 | int(b.Data[3]))
}

// RandomSleep .
func RandomSleep(done done.Done) {
	Sleep(time.Duration(101+rand.Intn(900))*time.Millisecond, done)
}

// Sleep .
func Sleep(d time.Duration, done done.Done) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-done.WaitCh():
	}
}
