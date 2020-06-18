package lockgate

import (
	"fmt"
	"os"
	"sync"

	"github.com/google/uuid"

	"github.com/werf/lockgate/pkg/file_lock"
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

	if l, hasKey := locker.locks[lockHandle.UUID]; hasKey {
		return l
	}

	locker.locks[lockHandle.UUID] = file_lock.NewFileLock(lockHandle.LockName, locker.LocksDir)
	return locker.locks[lockHandle.UUID]
}

func (locker *FileLocker) getLock(lockHandle LockHandle) file_lock.LockObject {
	locker.mux.Lock()
	defer locker.mux.Unlock()
	return locker.locks[lockHandle.UUID]
}

func (locker *FileLocker) Acquire(lockName string, opts AcquireOptions) (bool, LockHandle, error) {
	lockHandle := LockHandle{
		UUID:     uuid.New().String(),
		LockName: lockName,
	}

	lock := locker.newLock(lockHandle)

	var wrappedOnWaitFunc func(doWait func() error) error
	if opts.OnWaitFunc != nil {
		wrappedOnWaitFunc = func(doWait func() error) error {
			return opts.OnWaitFunc(lockName, doWait)
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
		return fmt.Errorf("unknown id %q for lock %q", lockHandle.UUID, lockHandle.LockName)
	} else {
		return lock.Unlock()
	}
}
