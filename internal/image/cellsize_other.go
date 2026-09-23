//go:build !unix

package img

func termCellPx() (int, int, bool) { return 0, 0, false }
