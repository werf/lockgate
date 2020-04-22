package file_lock

import (
	"fmt"
	"time"

	"github.com/gofrs/flock"
)

type fileLocker struct {
	baseLocker

	FileLock    *FileLock
	lockHandler *flock.Flock
}

func (locker *fileLocker) tryLock() (bool, error) {
	if locker.lockHandler == nil {
		panic("lockHandler is not set")
	}
	if locker.ReadOnly {
		return locker.lockHandler.TryRLock()
	}
	return locker.lockHandler.TryLock()
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
