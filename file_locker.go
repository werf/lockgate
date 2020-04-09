package lockgate

import (
	"fmt"
	"os"

	"github.com/flant/lockgate/file_lock"
)

type FileLockgate struct {
	LocksDir string
	Locks    map[string]file_lock.LockObject
}

func NewFileLockgate(locksDir string) (*FileLockgate, error) {
	if err := os.MkdirAll(locksDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create dir %s: %s", locksDir, err)
	}

	return &FileLockgate{
		LocksDir: locksDir,
		Locks:    make(map[string]file_lock.LockObject),
	}, nil
}

func (locker *FileLockgate) getLock(name string) file_lock.LockObject {
	if l, hasKey := locker.Locks[name]; hasKey {
		return l
	}

	locker.Locks[name] = file_lock.NewFileLock(name, locker.LocksDir)
	return locker.Locks[name]
}

func (locker *FileLockgate) Acquire(name string, opts AcquireOptions) (bool, error) {
	lock := locker.getLock(name)

	if opts.NonBlocking {
		return lock.TryLock(opts.Shared)
	} else {
		return true, lock.Lock(
			getTimeout(opts), opts.Shared,
			opts.OnWaitFunc,
		)
	}
}

func (locker *FileLockgate) Release(name string) error {
	if _, hasKey := locker.Locks[name]; !hasKey {
		panic(fmt.Sprintf("lock %q has not been acquired", name))
	}

	lock := locker.getLock(name)
	return lock.Unlock()
}

//func onWait(name string, doWait func() error) error {
//	logProcessMsg := fmt.Sprintf("Waiting for locked resource %q", name)
//	return logboek.LogProcessInline(logProcessMsg, logboek.LogProcessInlineOptions{}, func() error {
//		return doWait()
//	})
//}
