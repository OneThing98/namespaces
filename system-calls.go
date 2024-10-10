package namespaces

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	TIOCGPTN   = 0x80045430
	TIOCSPTLCK = 0x40045431
)

func UnlockPT(f *os.File) error {
	var u int
	return ioctl(f.Fd(), TIOCSPTLCK, uintptr(unsafe.Pointer(&u)))
}

func PTSName(f *os.File) (string, error) {
	var n int
	if err := ioctl(f.Fd(), TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}
	return fmt.Sprintf("/dev/pts/%d", n), nil
}

func ioctl(fd uintptr, flag, data uintptr) error {
	if _, _, err := unix.Syscall(unix.SYS_IOCTL, fd, flag, data); err != 0 {
		return err
	}
	return nil
}
