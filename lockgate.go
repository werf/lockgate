package lockgate

import "time"

// Locker is an abstract interface to interact with the locker.
// Locker implementation is always thread safe so it is possible
// to use a single Locker in multiple goroutines.
//
// Note that LockHandle objects should be managed by the user manually to
// acquire multiple locks from the same process: to release a lock user must pass
// the same LockHandle object that was given by Acquire method to the Release method.
type Locker interface {
	Acquire(lockName string, opts AcquireOptions) (bool, LockHandle, error)
	Release(lock LockHandle) error
}

type LockHandle struct {
	UUID     string
	LockName string
}

type AcquireOptions struct {
	NonBlocking bool
	Timeout     time.Duration
	Shared      bool

	OnWaitFunc      func(lock LockHandle, doWait func() error) error
	OnLostLeaseFunc func(lock LockHandle) error
}

func WithAcquire(locker Locker, lockName string, opts AcquireOptions, f func(acquired bool) error) (resErr error) {
	if acquired, lock, err := locker.Acquire(lockName, opts); err != nil {
		return err
	} else {
		if acquired {
			defer func() {
				if err := locker.Release(lock); err != nil {
					if resErr == nil {
						resErr = err
					}
				}
			}()
		}

		resErr = f(acquired)
	}

	return
}
