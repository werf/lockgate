package file_lock

import (
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

// maxAcquireAttempts bounds retries when the lock file keeps getting unlinked
// concurrently (by GCLockFileDir) right after we open it. In practice one retry
// is enough because a freshly created lock file is younger than GC's minAge.
const maxAcquireAttempts = 10

type fileLocker struct {
	baseLocker

	FileLock    *FileLock
	lockHandler *flock.Flock
}

func (locker *fileLocker) tryLock() (bool, error) {
	if locker.lockHandler == nil {
		panic("lockHandler is not set")
	}

	for attempt := 0; attempt < maxAcquireAttempts; attempt++ {
		var locked bool
		var err error
		if locker.ReadOnly {
			locked, err = locker.lockHandler.TryRLock()
		} else {
			locked, err = locker.lockHandler.TryLock()
		}
		if err != nil {
			return false, err
		}
		if !locked {
			return false, nil
		}

		// Inode-safe check: GC may have unlinked the lock file between our open
		// and flock, leaving us holding a dead inode while a new file with the
		// same name gets created and locked by someone else. Detect that and
		// retry on a freshly opened file so two processes never hold the "same"
		// logical lock at once.
		alive, err := lockFileIsAlive(locker.lockHandler)
		if err != nil {
			_ = locker.lockHandler.Unlock()
			return false, err
		}
		if alive {
			return true, nil
		}

		_ = locker.lockHandler.Unlock()
		locker.lockHandler = flock.New(locker.FileLock.LockFilePath())
	}

	return false, fmt.Errorf("unable to acquire lock for %s: lock file kept being removed concurrently", locker.FileLock.LockFilePath())
}

func (locker *fileLocker) TryLock() (bool, error) {
	locker.lockHandler = flock.New(locker.FileLock.LockFilePath())

	locked, err := locker.tryLock()
	if err != nil {
		return false, fmt.Errorf("error trying to lock file %s: %s", locker.FileLock.LockFilePath(), err)
	}

	return locked, nil
}

func (locker *fileLocker) Lock() error {
	locker.lockHandler = flock.New(locker.FileLock.LockFilePath())

	locked, err := locker.tryLock()
	if err != nil {
		return fmt.Errorf("error trying to lock file %s: %s", locker.FileLock.LockFilePath(), err)
	}

	if !locked {
		if locker.OnWaitFunc != nil {
			return locker.OnWaitFunc(func() error {
				return locker.pollLock()
			})
		} else {
			return locker.pollLock()
		}
	}

	return nil
}

func (locker *fileLocker) pollLock() error {
	flockRes := make(chan error)
	cancelPoll := make(chan bool)

	go func() {
		ticker := time.NewTicker(time.Millisecond * 500)

	PollFlock:
		for {
			select {
			case <-ticker.C:
				locked, err := locker.tryLock()
				if err != nil {
					flockRes <- fmt.Errorf("error trying to lock file %q while polling for lock: %s", locker.FileLock.LockFilePath(), err)
					break PollFlock
				}
				if locked {
					flockRes <- nil
					break PollFlock
				}
			case <-cancelPoll:
				break PollFlock
			}
		}
	}()

	if locker.Timeout != 0 {
		select {
		case err := <-flockRes:
			return err
		case <-time.After(locker.Timeout):
			cancelPoll <- true
			return fmt.Errorf("%q file lock timeout %s expired", locker.FileLock.LockFilePath(), locker.Timeout)
		}
	} else {
		select {
		case err := <-flockRes:
			return err
		}
	}
}

func (locker *fileLocker) Unlock() error {
	if err := locker.lockHandler.Unlock(); err != nil {
		return fmt.Errorf("error unlocking %q: %s", locker.FileLock.LockFilePath(), err)
	}
	locker.lockHandler = nil

	return nil
}
