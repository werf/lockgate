package lockgate

import (
	"fmt"
	"os"
	"sync"

	"github.com/google/uuid"

	"github.com/flant/lockgate/file_lock"
)

type FileLocker struct {
	LocksDir string

	mux   sync.Mutex
	locks map[string]file_lock.LockObject
}

func NewFileLocker(locksDir string) (*FileLocker, error) {
	if err := os.MkdirAll(locksDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create dir %s: %s", locksDir, err)
	}

	return &FileLocker{
		LocksDir: locksDir,
		locks:    make(map[string]file_lock.LockObject),
	}, nil
}

func (locker *FileLocker) newLock(lockHandle LockHandle) file_lock.LockObject {
	locker.mux.Lock()
	defer locker.mux.Unlock()

	if l, hasKey := locker.locks[lockHandle.ID]; hasKey {
		return l
	}

	locker.locks[lockHandle.ID] = file_lock.NewFileLock(lockHandle.LockName, locker.LocksDir)
	return locker.locks[lockHandle.ID]
}

func (locker *FileLocker) getLock(lockHandle LockHandle) file_lock.LockObject {
	locker.mux.Lock()
	defer locker.mux.Unlock()
	return locker.locks[lockHandle.ID]
}

func (locker *FileLocker) Acquire(lockName string, opts AcquireOptions) (bool, LockHandle, error) {
	lockHandle := LockHandle{
		ID:       uuid.New().String(),
		LockName: lockName,
	}

	lock := locker.newLock(lockHandle)

	var wrappedOnWaitFunc func(doWait func() error) error
	if opts.OnWaitFunc != nil {
		wrappedOnWaitFunc = func(doWait func() error) error {
			return opts.OnWaitFunc(lockHandle, doWait)
		}
	}

	if opts.NonBlocking {
		acquired, err := lock.TryLock(opts.Shared)
		return acquired, lockHandle, err
	} else {
		return true, lockHandle, lock.Lock(opts.Timeout, opts.Shared, wrappedOnWaitFunc)
	}
}

func (locker *FileLocker) Release(lockHandle LockHandle) error {
	if lock := locker.getLock(lockHandle); lock == nil {
		return fmt.Errorf("unknown id %q for lock %q", lockHandle.ID, lockHandle.LockName)
	} else {
		return lock.Unlock()
	}
}
