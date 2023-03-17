package distributed_locker

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/werf/lockgate"
)

type DistributedLocker struct {
	mux               sync.Mutex
	leaseRenewWorkers map[string]*LeaseRenewWorkerDescriptor

	Backend DistributedLockerBackend
}

type LeaseRenewWorkerDescriptor struct {
	DoneChan           chan struct{}
	SharedLeaseCounter int64
}

func NewDistributedLocker(backend DistributedLockerBackend) *DistributedLocker {
	return &DistributedLocker{
		Backend:           backend,
		leaseRenewWorkers: make(map[string]*LeaseRenewWorkerDescriptor),
	}
}

func (l *DistributedLocker) Acquire(lockName string, opts lockgate.AcquireOptions) (bool, lockgate.LockHandle, error) {
	debug("(acquire %q) opts=%#v", lockName, opts)
	return l.acquire(lockName, opts, true, time.Now())
}

func (l *DistributedLocker) acquire(lockName string, opts lockgate.AcquireOptions, shouldCallOnWait bool, startedAcquireAt time.Time) (bool, lockgate.LockHandle, error) {
RETRY_ACQUIRE:
	if opts.Timeout != 0 {
		if time.Now().After(startedAcquireAt.Add(opts.Timeout)) {
			return false, lockgate.LockHandle{}, fmt.Errorf("timeout")
		}
	}

	if lockHandle, err := l.Backend.Acquire(lockName, AcquireOptions{Shared: opts.Shared}); IsErrShouldWait(err) {
		if opts.NonBlocking {
			debug("(acquire %q) non blocking acquire done: lock not taken!", lockName)
			return false, lockgate.LockHandle{}, nil
		}

		debug("(acquire %q) poll lock: will retry in %d seconds", lockName, DistributedLockPollRetryPeriodSeconds)
		debug("(acquire %q) ---", lockName)

		if opts.OnWaitFunc != nil && shouldCallOnWait {
			var acquireLocked bool
			var acquireHandle lockgate.LockHandle
			var acquireErr error

			if err := opts.OnWaitFunc(lockName, func() error {
				time.Sleep(DistributedLockPollRetryPeriodSeconds * time.Second)
				acquireLocked, acquireHandle, acquireErr = l.acquire(lockName, opts, false, startedAcquireAt)
				return acquireErr
			}); err != nil {
				return acquireLocked, acquireHandle, err
			}

			return acquireLocked, acquireHandle, acquireErr
		} else {
			time.Sleep(DistributedLockPollRetryPeriodSeconds * time.Second)
			goto RETRY_ACQUIRE
		}
	} else if err != nil {
		return false, lockgate.LockHandle{}, err
	} else {
		l.runLeaseRenewWorker(lockHandle, opts)
		return true, lockHandle, nil
	}
}

func (l *DistributedLocker) Release(handle lockgate.LockHandle) error {
	debug("(release lock %q) %#v", handle.LockName, handle)
	return l.release(handle)
}

func (l *DistributedLocker) release(handle lockgate.LockHandle) error {
	if err := l.stopLeaseRenewWorker(handle); err != nil {
		return err
	}

	if err := l.Backend.Release(handle); IsErrLockAlreadyLeased(err) || IsErrNoExistingLockLeaseFound(err) {
		// TODO: maybe should call OnLostLease handler func
		// TODO: which should be saved
		return err
	} else if err != nil {
		return err
	} else {
		return nil
	}
}

func (l *DistributedLocker) runLeaseRenewWorker(handle lockgate.LockHandle, opts lockgate.AcquireOptions) {
	debug("(runLeaseRenewWorker %q %q) before lock", handle.LockName, handle.UUID)
	l.mux.Lock()
	defer func() {
		debug("(runLeaseRenewWorker %q %q) unlock", handle.LockName, handle.UUID)
		l.mux.Unlock()
	}()
	debug("(runLeaseRenewWorker %q %q) after lock", handle.LockName, handle.UUID)

	if desc, hasKey := l.leaseRenewWorkers[handle.UUID]; !hasKey {
		desc := &LeaseRenewWorkerDescriptor{
			DoneChan:           make(chan struct{}, 0),
			SharedLeaseCounter: 1,
		}
		l.leaseRenewWorkers[handle.UUID] = desc
		go l.leaseRenewWorker(handle, opts, desc.DoneChan)
	} else {
		desc.SharedLeaseCounter++
	}
}

func (l *DistributedLocker) leaseRenewWorker(handle lockgate.LockHandle, opts lockgate.AcquireOptions, doneChan chan struct{}) {
	ticker := time.NewTicker(DistributedLockLeaseRenewPeriodSeconds * time.Second)
	defer ticker.Stop()

	var lastRenewAt time.Time

	for {
		select {
		case <-ticker.C:
			debug("(leaseRenewWorker %q %q) tick!", handle.LockName, handle.UUID)

			// Throttle lease renew procedure, do not renew lease more than once in distributedLockLeaseRenewPeriodSeconds seconds period
			if time.Now().Sub(lastRenewAt) < DistributedLockLeaseRenewPeriodSeconds*time.Second {
				debug("(leaseRenewWorker %q %q) skip, last lease renew was at %s", handle.LockName, handle.UUID, lastRenewAt.String())
				continue
			}

			if !l.isLeaseRenewWorkerActive(handle) {
				debug("(leaseRenewWorker %q %q) already stopped, ignore check", handle.LockName, handle.UUID)
				continue
			}

			debug("(leaseRenewWorker %q %q) do lease renew", handle.LockName, handle.UUID)

			if err := l.Backend.RenewLease(handle); IsErrLockAlreadyLeased(err) || IsErrNoExistingLockLeaseFound(err) {
				fmt.Fprintf(os.Stderr, "ERROR: lost lease %s for lock %q\n", handle.UUID, handle.LockName)
				if opts.OnLostLeaseFunc != nil {
					if err := opts.OnLostLeaseFunc(handle); err != nil {
						fmt.Fprintf(os.Stderr, "ERROR: lost lease handler error: %s\n", err)
					}
				}
				return
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: lock server respond with an error: %s\n", err)
			} else {
				lastRenewAt = time.Now()
			}

		case <-doneChan:
			debug("(leaseRenewWorker %q %q) stopped!", handle.LockName, handle.UUID)
			return
		}
	}
}

func (l *DistributedLocker) isLeaseRenewWorkerActive(handle lockgate.LockHandle) bool {
	debug("(isLeaseRenewWorkerActive %q %q) before lock", handle.LockName, handle.UUID)
	l.mux.Lock()
	defer func() {
		debug("(isLeaseRenewWorkerActive%q %q) unlock", handle.LockName, handle.UUID)
		l.mux.Unlock()
	}()
	debug("(isLeaseRenewWorkerActive %q %q) after lock", handle.LockName, handle.UUID)

	_, hasKey := l.leaseRenewWorkers[handle.UUID]
	return hasKey
}

func (l *DistributedLocker) stopLeaseRenewWorker(handle lockgate.LockHandle) error {
	debug("(stopLeaseRenewWorker %q %q) before lock", handle.LockName, handle.UUID)
	l.mux.Lock()
	isLocked := true
	unlockFunc := func() {
		debug("(stopLeaseRenewWorker %q %q) unlock", handle.LockName, handle.UUID)
		if isLocked {
			l.mux.Unlock()
			isLocked = false
		}
	}
	defer unlockFunc()
	debug("(stopLeaseRenewWorker %q %q) after lock", handle.LockName, handle.UUID)

	if desc, hasKey := l.leaseRenewWorkers[handle.UUID]; !hasKey {
		return fmt.Errorf("unknown id %q for lock %q", handle.UUID, handle.LockName)
	} else {
		desc.SharedLeaseCounter--
		if desc.SharedLeaseCounter == 0 {
			delete(l.leaseRenewWorkers, handle.UUID)
			unlockFunc()

			debug("(stopLeaseRenewWorker %q %q) before DoneChan signal", handle.LockName, handle.UUID)
			desc.DoneChan <- struct{}{}
			debug("(stopLeaseRenewWorker %q %q) after DoneChan signal, before DoneChan close", handle.LockName, handle.UUID)
			close(desc.DoneChan)
			debug("(stopLeaseRenewWorker %q %q) after DoneChan close", handle.LockName, handle.UUID)
		}
	}

	return nil
}
