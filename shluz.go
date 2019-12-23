package shluz

import (
	"fmt"
	"os"
	"time"

	"github.com/flant/logboek"
)

var (
	LocksDir       string
	Locks          map[string]LockObject
	DefaultTimeout = 24 * time.Hour
)

func Init(locksDir string) error {
	Locks = make(map[string]LockObject)
	LocksDir = locksDir

	err := os.MkdirAll(LocksDir, 0755)
	if err != nil {
		return fmt.Errorf("cannot initialize locks dir: %s", err)
	}

	return nil
}

type LockOptions struct {
	Timeout  time.Duration
	ReadOnly bool
}

func Lock(name string, opts LockOptions) error {
	lock := getLock(name)

	return lock.Lock(
		getTimeout(opts), opts.ReadOnly,
		func(doWait func() error) error { return onWait(name, doWait) },
	)
}

type TryLockOptions struct {
	ReadOnly bool
}

func TryLock(name string, opts TryLockOptions) (bool, error) {
	lock := getLock(name)
	return lock.TryLock(opts.ReadOnly)
}

func Unlock(name string) error {
	if _, hasKey := Locks[name]; !hasKey {
		return fmt.Errorf("no such lock %q found", name)
	}

	lock := getLock(name)

	return lock.Unlock()
}

func WithLock(name string, opts LockOptions, f func() error) error {
	lock := getLock(name)

	return lock.WithLock(
		getTimeout(opts), opts.ReadOnly,
		func(doWait func() error) error { return onWait(name, doWait) },
		f,
	)
}

func onWait(name string, doWait func() error) error {
	logProcessMsg := fmt.Sprintf("Waiting for locked resource %q", name)
	return logboek.LogProcessInline(logProcessMsg, logboek.LogProcessInlineOptions{}, func() error {
		return doWait()
	})
}

func getTimeout(opts LockOptions) time.Duration {
	if opts.Timeout != 0 {
		return opts.Timeout
	}
	return DefaultTimeout
}

func getLock(name string) LockObject {
	if l, hasKey := Locks[name]; hasKey {
		return l
	}

	Locks[name] = NewFileLock(name, LocksDir)

	return Locks[name]
}

type LockObject interface {
	GetName() string
	TryLock(readOnly bool) (bool, error)
	Lock(timeout time.Duration, readOnly bool, onWait func(doWait func() error) error) error
	Unlock() error
	WithLock(timeout time.Duration, readOnly bool, onWait func(doWait func() error) error, f func() error) error
}
