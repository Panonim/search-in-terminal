//go:build unix

package img

import (
	"os"

	"golang.org/x/sys/unix"
)

// termCellPx reads the cell size from the tty's window size, which is a local ioctl, not a terminal query.
func termCellPx() (int, int, bool) {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 || ws.Row == 0 || ws.Xpixel == 0 || ws.Ypixel == 0 {
		return 0, 0, false
	}
	return int(ws.Xpixel / ws.Col), int(ws.Ypixel / ws.Row), true
}
