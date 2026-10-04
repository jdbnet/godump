package backup

import (
	"os"
	"syscall"
)

// POSIX_FADV_DONTNEED
const posixFadvDontNeed = 4

func dropFileCache(f *os.File) {
	if f == nil {
		return
	}
	_ = f.Sync()
	_, _, _ = syscall.Syscall6(syscall.SYS_FADVISE64, f.Fd(), 0, 0, posixFadvDontNeed, 0, 0)
}
