//go:build windows

package file_lock

import "github.com/gofrs/flock"

// lockFileIsAlive is a no-op on Windows: the OS forbids unlinking a file that
// is open/locked, so GCLockFileDir can never remove a lock file out from under
// a live holder and the POSIX dead-inode race does not apply.
func lockFileIsAlive(_ *flock.Flock) (bool, error) {
	return true, nil
}
