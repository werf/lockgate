package distributed_locker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/werf/lockgate"
	"github.com/werf/lockgate/pkg/distributed_locker/optimistic_locking_store"
	"github.com/werf/lockgate/pkg/util"
)

type OptimisticLockingStorageBasedBackend struct {
	Store optimistic_locking_store.OptimisticLockingStore
}

func NewOptimisticLockingStorageBasedBackend(store optimistic_locking_store.OptimisticLockingStore) *OptimisticLockingStorageBasedBackend {
	return &OptimisticLockingStorageBasedBackend{
		Store: store,
	}
}

func (handler *OptimisticLockingStorageBasedBackend) keyName(lockName string) string {
	return fmt.Sprintf("lockgate.io/%s", util.Sha3_224Hash(lockName))
}

func (backend *OptimisticLockingStorageBasedBackend) Acquire(lockName string, opts AcquireOptions) (lockgate.LockHandle, error) {
	storeKeyName := backend.keyName(lockName)

RETRY_ACQUIRE:
	if value, err := backend.Store.GetValue(storeKeyName); err != nil {
		return lockgate.LockHandle{}, fmt.Errorf("unable to get store value by name %s: %s", storeKeyName, err)
	} else {
		debug("(acquire lock %q) got record by key %s: %#v", lockName, storeKeyName, value)

		if oldLease, err := extractLockLeaseFromStoreValue(value); err != nil {
			return lockgate.LockHandle{}, fmt.Errorf("unable to extract lock lease record from data record by key %s: %s", storeKeyName, err)
		} else if oldLease != nil {
			debug("(acquire lock %q) oldLease -> %#v", lockName, oldLease)

			if time.Now().After(time.Unix(oldLease.ExpireAtTimestamp, 0)) {
				debug("(acquire lock %q) old lease expired, take over with the new lease!", lockName)

				newLease := NewLockLeaseRecord(lockName, opts.Shared)
				debug("(acquire lock %q) new lease: %#v", lockName, newLease)

				setLockLeaseIntoStoreValue(newLease, value)
				if err := backend.Store.PutValue(storeKeyName, value); optimistic_locking_store.IsErrRecordVersionChanged(err) {
					debug("(acquire lock %q update key %s optimistic locking error! Will retry acquire ...", lockName, storeKeyName)
					time.Sleep(DistributedOptimisticLockingRetryPeriodSeconds * time.Second)
					goto RETRY_ACQUIRE
				} else if err != nil {
					return lockgate.LockHandle{}, fmt.Errorf("unable to put store value by key %s: %s", storeKeyName, err)
				}

				return newLease.LockHandle, nil
			}

			if opts.Shared && oldLease.IsShared {
				oldLease.SharedHoldersCount++
				oldLease.ExpireAtTimestamp = time.Now().Unix() + DistributedLockLeaseTTLSeconds
				debug("(acquire lock %q) incremented shared holders counter for existing lease: %#v", lockName, oldLease)

				setLockLeaseIntoStoreValue(oldLease, value)
				if err := backend.Store.PutValue(storeKeyName, value); optimistic_locking_store.IsErrRecordVersionChanged(err) {
					debug("(acquire lock %q) update key %s optimistic locking error! Will retry acquire ...", lockName, storeKeyName)
					time.Sleep(DistributedOptimisticLockingRetryPeriodSeconds * time.Second)
					goto RETRY_ACQUIRE
				} else if err != nil {
					return lockgate.LockHandle{}, fmt.Errorf("unable to put store value by key %s: %s", storeKeyName, err)
				}

				return oldLease.LockHandle, nil
			}

			return lockgate.LockHandle{}, ErrShouldWait
		}

		newLease := NewLockLeaseRecord(lockName, opts.Shared)
		debug("(acquire lock %q) new lease: %#v", lockName, newLease)

		setLockLeaseIntoStoreValue(newLease, value)
		if err := backend.Store.PutValue(storeKeyName, value); optimistic_locking_store.IsErrRecordVersionChanged(err) {
			debug("(acquire lock %q update key %s optimistic locking error! Will retry acquire ...", lockName, storeKeyName)
			time.Sleep(DistributedOptimisticLockingRetryPeriodSeconds * time.Second)
			goto RETRY_ACQUIRE
		} else if err != nil {
			return lockgate.LockHandle{}, fmt.Errorf("unable to put store value by key %s: %s", storeKeyName, err)
		}

		return newLease.LockHandle, nil
	}
}

func (backend *OptimisticLockingStorageBasedBackend) RenewLease(handle lockgate.LockHandle) error {
	return backend.changeLease(handle, func(value *optimistic_locking_store.Value, lease *LockLeaseRecord) error {
		lease.ExpireAtTimestamp = time.Now().Unix() + DistributedLockLeaseTTLSeconds
		setLockLeaseIntoStoreValue(lease, value)
		return nil
	})
}

func (backend *OptimisticLockingStorageBasedBackend) Release(handle lockgate.LockHandle) error {
	return backend.changeLease(handle, func(value *optimistic_locking_store.Value, currentLease *LockLeaseRecord) error {
		currentLease.SharedHoldersCount--

		if currentLease.SharedHoldersCount == 0 {
			unsetLockLeaseFromStoreValue(value)
		} else {
			setLockLeaseIntoStoreValue(currentLease, value)
		}

		return nil
	})
}

func (backend *OptimisticLockingStorageBasedBackend) changeLease(lockHandle lockgate.LockHandle, changeFunc func(value *optimistic_locking_store.Value, currentLease *LockLeaseRecord) error) error {
	storeKeyName := backend.keyName(lockHandle.LockName)

RETRY_CHANGE:
	if value, err := backend.Store.GetValue(storeKeyName); err != nil {
		return err
	} else {
		debug("(change lock %q lease) get store value by key %s -> %#v", lockHandle.LockName, storeKeyName, value)

		if currentLease, err := extractLockLeaseFromStoreValue(value); err != nil {
			return err
		} else if currentLease == nil {
			return ErrNoExistingLockLeaseFound
		} else if currentLease.UUID != lockHandle.UUID {
			return ErrLockAlreadyLeased
		} else {
			if err := changeFunc(value, currentLease); err != nil {
				return err
			}
		}

		if err := backend.Store.PutValue(storeKeyName, value); optimistic_locking_store.IsErrRecordVersionChanged(err) {
			debug("(change lock %q lease) update store value by key %s optimistic locking error! Will retry change...", lockHandle.LockName, storeKeyName)
			time.Sleep(DistributedOptimisticLockingRetryPeriodSeconds * time.Second)
			goto RETRY_CHANGE
		} else if err != nil {
			return err
		}

		return nil
	}
}

func extractLockLeaseFromStoreValue(value *optimistic_locking_store.Value) (*LockLeaseRecord, error) {
	if value.Data == "" {
		return nil, nil
	}

	var lease *LockLeaseRecord
	if err := json.Unmarshal([]byte(value.Data), &lease); err != nil {
		return nil, err
	}
	return lease, nil
}

func setLockLeaseIntoStoreValue(lease *LockLeaseRecord, value *optimistic_locking_store.Value) {
	if data, err := json.Marshal(lease); err != nil {
		panic(fmt.Sprintf("json marshal %#v failed: %s", lease, err))
	} else {
		value.Data = string(data)
	}
}

func unsetLockLeaseFromStoreValue(value *optimistic_locking_store.Value) {
	value.Data = ""
}
