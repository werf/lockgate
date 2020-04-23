package file_lock

import (
	"path/filepath"
	"time"

	"github.com/flant/lockgate/pkg/util"
)

var (
	LegacyHashFunction = false
)

func NewFileLock(name string, locksDir string) LockObject {
	return &FileLock{BaseLock: BaseLock{Name: name}, LocksDir: locksDir}
}

type FileLock struct {
	BaseLock
	LocksDir string
	locker   *fileLocker
}

func (lock *FileLock) newLocker(timeout time.Duration, readOnly bool, onWaitFunc func(doWait func() error) error) *fileLocker {
	return &fileLocker{
		baseLocker: baseLocker{
			Timeout:    timeout,
			ReadOnly:   readOnly,
			OnWaitFunc: onWaitFunc,
		},
		FileLock: lock,
	}
}

func (lock *FileLock) TryLock(readOnly bool) (bool, error) {
	lock.locker = lock.newLocker(0, readOnly, nil)
	return lock.BaseLock.TryLock(lock.locker)
}

func (lock *FileLock) Lock(timeout time.Duration, readOnly bool, onWait func(doWait func() error) error) error {
	lock.locker = lock.newLocker(timeout, readOnly, onWait)
	return lock.BaseLock.Lock(lock.locker)
}

func (lock *FileLock) Unlock() error {
	if lock.locker == nil {
		return nil
	}

	err := lock.BaseLock.Unlock(lock.locker)
	if err != nil {
		return err
	}

	lock.locker = nil

	return nil
}

func (lock *FileLock) LockFilePath() string {
	// TODO: change to sha256 in the next major version
	// TODO: cannot change for now without breaking backward compatibility
	var fileName string
	if LegacyHashFunction {
		fileName = util.MurmurHash(lock.GetName())
	} else {
		fileName = util.Sha256Hash(lock.GetName())
	}

	return filepath.Join(lock.LocksDir, fileName)
}
