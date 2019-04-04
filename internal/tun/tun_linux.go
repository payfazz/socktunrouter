package tun

import (
	"bytes"
	"os"
	"unsafe"

	"github.com/payfazz/go-errors"
	"golang.org/x/sys/unix"
)

// Open .
func Open(name string) (*os.File, error) {
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, errors.Wrap(err)
	}

	var req struct {
		name  [0x10]byte
		flags uint16
		pad   [0x28 - 0x10 - 2]byte
	}

	copy(req.name[:], name)
	req.flags = unix.IFF_TUN | unix.IFF_NO_PI

	if _, _, err := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(fd), unix.TUNSETIFF, uintptr(unsafe.Pointer(&req)),
	); err != 0 {
		unix.Close(fd)
		return nil, errors.Wrap(err)
	}

	realNameLen := bytes.IndexByte(req.name[:], 0)
	if realNameLen < 0 {
		realNameLen = len(req.name)
	}

	realName := make([]byte, realNameLen)
	copy(realName, req.name[:realNameLen])

	return os.NewFile(uintptr(fd), string(realName)), nil
}
