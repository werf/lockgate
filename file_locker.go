package lockgate

import (
	"fmt"
	"os"

	"github.com/flant/lockgate/file_lock"
)

type FileLocker struct {
	LocksDir string
	Locks    map[string]file_lock.LockObject
}

func NewFileLocker(locksDir string) (*FileLocker, error) {
	if err := os.MkdirAll(locksDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create dir %s: %s", locksDir, err)
	}

	return &FileLocker{
		LocksDir: locksDir,
		Locks:    make(map[string]file_lock.LockObject),
	}, nil
}

func (locker *FileLocker) getLock(lockName string) file_lock.LockObject {
	if l, hasKey := locker.Locks[lockName]; hasKey {
		return l
	}

	locker.Locks[lockName] = file_lock.NewFileLock(lockName, locker.LocksDir)
	return locker.Locks[lockName]
}

func (locker *FileLocker) Acquire(lockName string, opts AcquireOptions) (bool, error) {
	lock := locker.getLock(lockName)

	var wrappedOnWaitFunc func(doWait func() error) error
	if opts.OnWaitFunc != nil {
		wrappedOnWaitFunc = func(doWait func() error) error {
			return opts.OnWaitFunc(lockName, doWait)
		}
	}

	if opts.NonBlocking {
		return lock.TryLock(opts.Shared)
	} else {
		return true, lock.Lock(opts.Timeout, opts.Shared, wrappedOnWaitFunc)
	}
}

func (locker *FileLocker) Release(lockName string) error {
	if _, hasKey := locker.Locks[lockName]; !hasKey {
		panic(fmt.Sprintf("lock %q has not been acquired", lockName))
	}

	lock := locker.getLock(lockName)
	return lock.Unlock()
}

//func onWait(lockName string, doWait func() error) error {
//	logProcessMsg := fmt.Sprintf("Waiting for locked resource %q", name)
//	return logboek.LogProcessInline(logProcessMsg, logboek.LogProcessInlineOptions{}, func() error {
//		return doWait()
//	})
//}
