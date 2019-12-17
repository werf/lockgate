package shluz

import (
	"time"
)

type locker interface {
	Lock() error
	Unlock() error
}

type baseLocker struct {
	Timeout  time.Duration
	ReadOnly bool
	OnWait   func(doWait func() error) error
}

func (locker *baseLocker) Lock() error {
	panic("not implemented")
}

func (locker *baseLocker) Unlock() error {
	panic("not implemented")
}

type BaseLock struct {
	Name        string
	ActiveLocks int
}

func (lock *BaseLock) GetName() string {
	return lock.Name
}

func (lock *BaseLock) Lock(l locker) error {
	if lock.ActiveLocks == 0 {
		err := l.Lock()
		if err != nil {
			return err
		}
	}

	lock.ActiveLocks += 1

	return nil
}

func (lock *BaseLock) Unlock(l locker) error {
	if lock.ActiveLocks == 0 {
		return nil
	}

	lock.ActiveLocks -= 1

	if lock.ActiveLocks == 0 {
		return l.Unlock()
	}

	return nil
}

func (lock *BaseLock) WithLock(locker locker, f func() error) (resErr error) {
	if err := lock.Lock(locker); err != nil {
		return err
	}

	defer func() {
		if err := lock.Unlock(locker); err != nil {
			if resErr == nil {
				resErr = err
			}
		}
	}()

	resErr = f()

	return
}