package file_locker

import (
	"fmt"
	"os"
	"sync"

	"github.com/google/uuid"

	"github.com/werf/lockgate"
	"github.com/werf/lockgate/pkg/file_lock"
)

type FileLocker struct {
	LocksDir string

	mux   sync.Mutex
	locks map[string]file_lock.LockObject
}

func NewFileLocker(locksDir string) (*FileLocker, error) {
	if err := os.MkdirAll(locksDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create dir %s: %s", locksDir, err)
	}

	return &FileLocker{
		LocksDir: locksDir,
		locks:    make(map[string]file_lock.LockObject),
	}, nil
}

func (l *FileLocker) newLock(lockHandle lockgate.LockHandle) file_lock.LockObject {
	l.mux.Lock()
	defer l.mux.Unlock()

	if l, hasKey := l.locks[lockHandle.UUID]; hasKey {
		return l
	}

	l.locks[lockHandle.UUID] = file_lock.NewFileLock(lockHandle.LockName, l.LocksDir)
	return l.locks[lockHandle.UUID]
}

func (l *FileLocker) getAndRemoveLock(lockHandle lockgate.LockHandle) file_lock.LockObject {
	l.mux.Lock()
	defer l.mux.Unlock()

	if lock, hasKey := l.locks[lockHandle.UUID]; hasKey {
		delete(l.locks, lockHandle.UUID)
		return lock
	}

	return nil
}

func (l *FileLocker) Acquire(lockName string, opts lockgate.AcquireOptions) (bool, lockgate.LockHandle, error) {
	lockHandle := lockgate.LockHandle{
		UUID:     uuid.New().String(),
		LockName: lockName,
	}

	lock := l.newLock(lockHandle)

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

func (l *FileLocker) Release(lockHandle lockgate.LockHandle) error {
	if lock := l.getAndRemoveLock(lockHandle); lock == nil {
		return fmt.Errorf("unknown id %q for lock %q", lockHandle.UUID, lockHandle.LockName)
	} else {
		return lock.Unlock()
	}
}
