//go:build !windows

package file_lock

import (
	"errors"
	"io/fs"
	"syscall"

	"github.com/gofrs/flock"
)

// lockFileIsAlive reports whether the locked file is still linked into the
// filesystem. flock.Stat() runs fstat on the held descriptor, so a link count
// of zero means the file was unlinked (e.g. by GCLockFileDir) after we opened
// it — we are holding a dead inode and must retry.
func lockFileIsAlive(fileLock *flock.Flock) (bool, error) {
	info, err := fileLock.Stat()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return st.Nlink > 0, nil
	}

	// Unknown platform stat shape: assume alive to avoid spurious retries.
	return true, nil
}
