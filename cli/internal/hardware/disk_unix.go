//go:build !windows

package hardware

import (
	"syscall"
)

// FreeSpaceBytes returns the free space in bytes of the disk containing the path.
func FreeSpaceBytes(path string) (uint64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, err
	}
	return uint64(stat.Bavail) * uint64(stat.Bsize), nil
}
