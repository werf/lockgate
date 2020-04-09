package lockgate

import "time"

type Locker interface {
	Acquire(lockName string, opts AcquireOptions) (bool, error)
	Release(lockName string) error
}

type AcquireOptions struct {
	NonBlocking bool
	Timeout     time.Duration
	Shared      bool

	OnWaitFunc      func(lockName string, doWait func() error) error
	OnLostLeaseFunc func(lockName string) error
}

func WithAcquire(locker Locker, lockName string, opts AcquireOptions, f func(acquired bool) error) (resErr error) {
	if acquired, err := locker.Acquire(lockName, opts); err != nil {
		return err
	} else {
		if acquired {
			defer func() {
				if err := locker.Release(lockName); err != nil {
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
