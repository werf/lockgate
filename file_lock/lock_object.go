package file_lock

import "time"

type LockObject interface {
	GetName() string
	TryLock(readOnly bool) (bool, error)
	Lock(timeout time.Duration, readOnly bool, onWait func(doWait func() error) error) error
	Unlock() error
}
