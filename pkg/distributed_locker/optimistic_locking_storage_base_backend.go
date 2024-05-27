package distributed_locker

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
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

			// If the lease is expired, create a new lease for the caller if
			// nobody else is waiting for they're first in line
			if time.Now().After(time.Unix(oldLease.ExpireAtTimestamp, 0)) {
				debug("(acquire lock %q) old lease expired, try to take over!", lockName)

				if newLease, err := backend.TakeIfOldest(oldLease.LockHandle, opts.AcquirerId); err != nil {
					debug("(acquire lock %q) failed %s", lockName, err.Error())
					return lockgate.LockHandle{}, err
				} else {
					debug("(acquire lock %q) new lease: %#v", lockName, newLease)
					return newLease.LockHandle, nil
				}
			}

			// If caller wants a shared lease, and the existing lease is shared, give it to them
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
			// Keep acquirer's place in line by updating the expiration date
			if err := backend.UpdateQueue(oldLease.LockHandle, opts.AcquirerId); err != nil {
				return lockgate.LockHandle{}, err
			}

			return lockgate.LockHandle{}, ErrShouldWait
		}

		// No existing lease; create a new one
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

// If the acquirer is first in line or nobody else is waiting, update the lease with a new UUID.
// If the acquirer is not first in line, they need to wait.
func (backend *OptimisticLockingStorageBasedBackend) TakeIfOldest(handle lockgate.LockHandle, acquirerId string) (*LockLeaseRecord, error) {
	var newLease *LockLeaseRecord
	err := backend.changeLease(handle, func(value *optimistic_locking_store.Value, currentLease *LockLeaseRecord) error {
		nextUp := &QueueMember{}
		now := time.Now().Unix()
		var expired []string
		for k, l := range currentLease.QueueMembers {
			if l.ExpireAtTimestamp < now {
				expired = append(expired, l.AcquirerId)
				continue
			}
			if k == acquirerId {
				// Update expiration to hold acquirer's place in line
				l.ExpireAtTimestamp = time.Now().Unix() + DistributedLockLeaseTTLSeconds
			}
			if nextUp.AcquiredAtTimestamp == 0 || l.AcquiredAtTimestamp < nextUp.AcquiredAtTimestamp {
				nextUp = l
			}
		}
		// Remove expired QueueMembers
		for _, key := range expired {
			delete(currentLease.QueueMembers, key)
		}

		if acquirerId != "" {
			if _, ok := currentLease.QueueMembers[acquirerId]; !ok {
				// Add new queue member
				currentLease.QueueMembers[acquirerId] = &QueueMember{
					AcquirerId:          acquirerId,
					AcquiredAtTimestamp: time.Now().Unix(),
					ExpireAtTimestamp:   time.Now().Unix() + DistributedLockLeaseTTLSeconds,
				}
			}
		}

		if nextUp.AcquirerId == "" || nextUp.AcquirerId == acquirerId {
			// Generate a new UUID for the new acquirer
			currentLease.UUID = uuid.New().String()
			currentLease.ExpireAtTimestamp = time.Now().Unix() + DistributedLockLeaseTTLSeconds
			currentLease.SharedHoldersCount = 1
			delete(currentLease.QueueMembers, acquirerId)
			newLease = currentLease
		}
		setLockLeaseIntoStoreValue(currentLease, value)

		return nil
	})
	if err != nil {
		return newLease, err
	}
	if newLease != nil {
		return newLease, nil
	}
	return newLease, ErrShouldWait
}

// Renew queue member's expiration
func (backend *OptimisticLockingStorageBasedBackend) UpdateQueue(handle lockgate.LockHandle, acquirerId string) error {
	if acquirerId == "" {
		return nil
	}
	return backend.changeLease(handle, func(value *optimistic_locking_store.Value, currentLease *LockLeaseRecord) error {
		if _, ok := currentLease.QueueMembers[acquirerId]; ok {
			currentLease.QueueMembers[acquirerId].ExpireAtTimestamp = time.Now().Unix() + DistributedLockLeaseTTLSeconds
		} else {
			currentLease.QueueMembers[acquirerId] = &QueueMember{
				AcquirerId:          acquirerId,
				AcquiredAtTimestamp: time.Now().Unix(),
				ExpireAtTimestamp:   time.Now().Unix() + DistributedLockLeaseTTLSeconds,
			}
		}

		setLockLeaseIntoStoreValue(currentLease, value)

		return nil
	})
}

func (backend *OptimisticLockingStorageBasedBackend) Release(handle lockgate.LockHandle) error {
	return backend.changeLease(handle, func(value *optimistic_locking_store.Value, currentLease *LockLeaseRecord) error {
		currentLease.SharedHoldersCount--
		now := time.Now().Unix()
		var expired []string
		for _, l := range currentLease.QueueMembers {
			if l.ExpireAtTimestamp < now {
				expired = append(expired, l.AcquirerId)
			}
		}
		for _, key := range expired {
			delete(currentLease.QueueMembers, key)
		}

		if currentLease.SharedHoldersCount == 0 {
			if len(currentLease.QueueMembers) == 0 {
				unsetLockLeaseFromStoreValue(value)
				return nil
			}
			// Expire the lease so the waiting QueueMembers can acquire it
			currentLease.ExpireAtTimestamp = time.Now().Add(-1 * time.Second).Unix()
		}
		setLockLeaseIntoStoreValue(currentLease, value)

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
