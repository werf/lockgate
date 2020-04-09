package file_lock

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/spaolacci/murmur3"
)

func NewFileLock(name string, locksDir string) LockObject {
	return &FileLock{BaseLock: BaseLock{Name: name}, LocksDir: locksDir}
}

type FileLock struct {
	BaseLock
	LocksDir string
	locker   *fileLocker
}

func (lock *FileLock) newLocker(timeout time.Duration, readOnly bool, onWaitFunc func(doWait func() error) error) *fileLocker {
	return &fileLocker{
		baseLocker: baseLocker{
			Timeout:    timeout,
			ReadOnly:   readOnly,
			OnWaitFunc: onWaitFunc,
		},
		FileLock: lock,
	}
}

func (lock *FileLock) TryLock(readOnly bool) (bool, error) {
	lock.locker = lock.newLocker(0, readOnly, nil)
	return lock.BaseLock.TryLock(lock.locker)
}

func (lock *FileLock) Lock(timeout time.Duration, readOnly bool, onWait func(doWait func() error) error) error {
	lock.locker = lock.newLocker(timeout, readOnly, onWait)
	return lock.BaseLock.Lock(lock.locker)
}

func (lock *FileLock) Unlock() error {
	if lock.locker == nil {
		return nil
	}

	err := lock.BaseLock.Unlock(lock.locker)
	if err != nil {
		return err
	}

	lock.locker = nil

	return nil
}

func (lock *FileLock) LockFilePath() string {
	fileName := MurmurHash(lock.GetName())
	return filepath.Join(lock.LocksDir, fileName)
}

func MurmurHash(args ...string) string {
	h32 := murmur3.New32()
	h32.Write([]byte(strings.Join(args, ":::")))
	sum := h32.Sum32()
	return fmt.Sprintf("%x", sum)
}
