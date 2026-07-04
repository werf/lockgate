package file_lock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// GCLockFileDir removes stale lock files from dirPath (recursively).
//
// Lock files accumulate because Unlock never deletes them. A file is removed
// only when all of these hold:
//   - its mtime is older than minAge (skips hot locks and makes the pass cheap
//     over huge directories: the mtime check happens before any lock attempt);
//   - GC can acquire an exclusive flock on it via TryLock (so it is not held by
//     anyone else right now).
//
// This is safe against the flock+unlink race only in combination with the
// inode-safe acquire path (see file_locker.go): a concurrent acquirer that
// opened the file just before GC unlinked it will detect the dead inode
// (Nlink==0) and retry on a freshly created file.
//
// Errors for individual files are collected and returned joined; a single bad
// file does not abort the whole pass. Files that disappear concurrently are
// ignored.
func GCLockFileDir(dirPath string, minAge time.Duration) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read dir %s: %w", dirPath, err)
	}

	var errs []error
	for _, entry := range entries {
		path := filepath.Join(dirPath, entry.Name())

		if entry.IsDir() {
			if err := GCLockFileDir(path, minAge); err != nil {
				errs = append(errs, err)
			}
			continue
		}

		info, err := entry.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			errs = append(errs, fmt.Errorf("stat %s: %w", path, err))
			continue
		}

		// Cheap pre-filter before touching any locks: keep recent (hot) files.
		if time.Since(info.ModTime()) < minAge {
			continue
		}

		if err := safeDeleteLockFile(path); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// safeDeleteLockFile removes a single lock file only if GC can take its
// exclusive flock. If the file is held by someone else (TryLock returns false),
// it is left in place. On Windows removing an open/locked file may fail; such
// failures are treated as "keep it" and do not error the pass.
func safeDeleteLockFile(path string) error {
	fileLock := flock.New(path)

	locked, err := fileLock.TryLock()
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("try lock %s: %w", path, err)
	}
	if !locked {
		// Held by another party — must not delete.
		return nil
	}
	defer fileLock.Unlock()

	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		// e.g. Windows: cannot remove a file that is open/locked. Keep it.
		return nil
	}

	return nil
}
