//go:build !linux

package backup

import "os"

func dropFileCache(f *os.File) {}
