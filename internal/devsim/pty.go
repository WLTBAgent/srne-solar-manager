package devsim

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// OpenPTY creates a pseudo-terminal pair and returns the master end
// (to serve Modbus on) and the slave path (to point a client at).
// The slave line discipline is switched to raw mode so binary Modbus
// frames pass through untouched and are not echoed back.
func OpenPTY() (master *os.File, slavePath string, err error) {
	m, err := os.OpenFile("/dev/ptmx", os.O_RDWR|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, "", fmt.Errorf("open /dev/ptmx: %w", err)
	}
	defer func() {
		if err != nil {
			m.Close()
		}
	}()

	n, err := unix.IoctlGetUint32(int(m.Fd()), unix.TIOCGPTN)
	if err != nil {
		return nil, "", fmt.Errorf("TIOCGPTN: %w", err)
	}
	if err := unix.IoctlSetPointerInt(int(m.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		return nil, "", fmt.Errorf("unlock pty: %w", err)
	}
	slavePath = fmt.Sprintf("/dev/pts/%d", n)

	t, err := unix.IoctlGetTermios(int(m.Fd()), unix.TCGETS)
	if err != nil {
		return nil, "", fmt.Errorf("TCGETS: %w", err)
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(int(m.Fd()), unix.TCSETS, t); err != nil {
		return nil, "", fmt.Errorf("TCSETS: %w", err)
	}

	return m, slavePath, nil
}
