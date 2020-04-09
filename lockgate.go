package lockgate

import "time"

type Locker interface {
	Acquire(name string, opts AcquireOptions) (bool, error)
	Release(name string) error
}

type AcquireOptions struct {
	NonBlocking bool
	Timeout     time.Duration
	Shared      bool

	OnWaitFunc      func(doWait func() error) error
	OnLostLeaseFunc func() error
}

const (
	DefaultTimeout = 24 * time.Hour
)

func getTimeout(opts AcquireOptions) time.Duration {
	if opts.Timeout != 0 {
		return opts.Timeout
	}
	return DefaultTimeout
}

func WithAcquire(locker Locker, name string, opts AcquireOptions, f func(acquired bool) error) (resErr error) {
	if acquired, err := locker.Acquire(name, opts); err != nil {
		return err
	} else {
		if acquired {
			defer func() {
				if err := locker.Release(name); err != nil {
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
