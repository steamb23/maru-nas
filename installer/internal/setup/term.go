package setup

import (
	"syscall"
	"unsafe"
)

// isTerminal은 fd가 터미널인지다. /dev/null 같은 문자 장치와 구분하려고 터미널 속성(TCGETS)을 읽어 본다.
func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
