package lockgate

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	default_errors "errors"

	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

const (
	lockLeaseTTLSeconds         = 10
	lockPollRetryPeriodSeconds  = 1
	lockLeaseRenewPeriodSeconds = 3
)

var (
	ErrLockAlreadyLeased        = default_errors.New("lock already leased")
	ErrNoExistingLockLeaseFound = default_errors.New("no existing lock lease found")
)

type KubernetesLocker struct {
	KubernetesInterface dynamic.Interface
	GVR                 schema.GroupVersionResource
	ResourceName        string
	Namespace           string

	mux               sync.Mutex
	leaseRenewWorkers map[string]chan struct{}
}

type LockLeaseRecord struct {
	LockHandle
	ExpiredAtTimestamp int64
}

func NewKubernetesLocker(kubernetesInterface dynamic.Interface, gvr schema.GroupVersionResource, resourceName string, namespace string) *KubernetesLocker {
	return &KubernetesLocker{
		KubernetesInterface: kubernetesInterface,
		GVR:                 gvr,
		ResourceName:        resourceName,
		Namespace:           namespace,
		leaseRenewWorkers:   make(map[string]chan struct{}),
	}
}

func (locker *KubernetesLocker) Acquire(lockName string, opts AcquireOptions) (bool, LockHandle, error) {
	return locker.acquire(lockName, opts)
}

func (locker *KubernetesLocker) Release(lockHandle LockHandle) error {
	return locker.release(lockHandle)
}

func (locker *KubernetesLocker) getResource() (*unstructured.Unstructured, error) {
	var err error
	var obj *unstructured.Unstructured

	if locker.Namespace == "" {
		obj, err = locker.KubernetesInterface.Resource(locker.GVR).Get(locker.ResourceName, metav1.GetOptions{})
	} else {
		obj, err = locker.KubernetesInterface.Resource(locker.GVR).Namespace(locker.Namespace).Get(locker.ResourceName, metav1.GetOptions{})
	}

	if err != nil {
		return obj, fmt.Errorf("cannot get %s by name %q: %s", locker.GVR.String(), locker.ResourceName, err)
	}
	return obj, err
}

func (locker *KubernetesLocker) updateResource(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var err error
	var newObj *unstructured.Unstructured

	if locker.Namespace == "" {
		newObj, err = locker.KubernetesInterface.Resource(locker.GVR).Update(obj, metav1.UpdateOptions{})
	} else {
		newObj, err = locker.KubernetesInterface.Resource(locker.GVR).Namespace(locker.Namespace).Update(obj, metav1.UpdateOptions{})
	}

	if errors.IsAlreadyExists(err) || err == nil {
		return newObj, err
	} else {
		return newObj, fmt.Errorf("cannot update %s by name %s: %s", locker.GVR.String(), locker.ResourceName, err)
	}
}

func (locker *KubernetesLocker) newLockLeaseRecord(lockName string) *LockLeaseRecord {
	return &LockLeaseRecord{
		LockHandle:         LockHandle{ID: uuid.New().String(), LockName: lockName},
		ExpiredAtTimestamp: time.Now().Unix() + lockLeaseTTLSeconds,
	}
}

func (locker *KubernetesLocker) objectLockLeaseAnnotationName(lockName string) string {
	return fmt.Sprintf("lockgate.io/%s", lockName)
}

func (locker *KubernetesLocker) extractObjectLockLease(obj *unstructured.Unstructured, lockName string) (*LockLeaseRecord, error) {
	annots := obj.GetAnnotations()
	if data, hasKey := annots[locker.objectLockLeaseAnnotationName(lockName)]; hasKey {
		var lease *LockLeaseRecord
		if err := json.Unmarshal([]byte(data), &lease); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: invalid %s annotation value in %s/%s: %s", locker.objectLockLeaseAnnotationName(lockName), obj.GetKind(), locker.ResourceName, err)
			return nil, nil
		}
		return lease, nil
	}
	return nil, nil
}

func (locker *KubernetesLocker) setObjectLockLease(obj *unstructured.Unstructured, lease *LockLeaseRecord) {
	if data, err := json.Marshal(lease); err != nil {
		panic(fmt.Sprintf("json marshal %#v failed: %s", lease, err))
	} else {
		annots := obj.GetAnnotations()
		if annots == nil {
			annots = make(map[string]string)
		}
		annots[locker.objectLockLeaseAnnotationName(lease.LockName)] = string(data)
		obj.SetAnnotations(annots)
	}
}

func (locker *KubernetesLocker) unsetObjectLockLease(obj *unstructured.Unstructured, lockName string) {
	annots := obj.GetAnnotations()
	delete(annots, locker.objectLockLeaseAnnotationName(lockName))
	obj.SetAnnotations(annots)
}

func (locker *KubernetesLocker) acquire(lockName string, opts AcquireOptions) (bool, LockHandle, error) {
RETRY_ACQUIRE:

	if obj, err := locker.getResource(); err != nil {
		return false, LockHandle{}, err
	} else {
		debug("(acquire lock %q) getResource -> %#v", lockName, obj)

		if oldLease, err := locker.extractObjectLockLease(obj, lockName); err != nil {
			return false, LockHandle{}, err
		} else if oldLease != nil {
			debug("(acquire lock %q)  oldLease -> %#v", lockName, oldLease)

			if time.Now().After(time.Unix(oldLease.ExpiredAtTimestamp, 0)) {
				debug("(acquire lock %q)  old lease expired, take over with the new lease!", lockName)

				newLease := locker.newLockLeaseRecord(lockName)
				debug("(acquire lock %q)  new lease: %#v", lockName, newLease)

				locker.setObjectLockLease(obj, newLease)
				if _, err := locker.updateResource(obj); errors.IsConflict(err) {
					debug("(acquire lock %q)  update %#v conflict! will retry acquire", lockName, obj)
					goto RETRY_ACQUIRE
				} else if err != nil {
					return false, LockHandle{}, err
				}
				locker.runLeaseRenewWorker(newLease.LockHandle, opts)
				return true, newLease.LockHandle, nil
			}

			if opts.NonBlocking {
				debug("(acquire lock %q)  non blocking acquire done: lock not taken!", lockName)
				return false, LockHandle{}, nil
			}

			debug("(acquire lock %q)  poll lock: will retry in %d seconds", lockName, lockPollRetryPeriodSeconds)
			debug("(acquire lock %q)  ---", lockName)
			time.Sleep(lockPollRetryPeriodSeconds * time.Second)
			goto RETRY_ACQUIRE
		}

		newLease := locker.newLockLeaseRecord(lockName)
		debug("(acquire lock %q)  new lease: %#v", lockName, newLease)

		locker.setObjectLockLease(obj, newLease)
		if _, err := locker.updateResource(obj); errors.IsConflict(err) {
			debug("(acquire lock %q)  update %#v conflict! Will retry acquire...", lockName, obj)
			goto RETRY_ACQUIRE
		} else if err != nil {
			return false, LockHandle{}, err
		}
		locker.runLeaseRenewWorker(newLease.LockHandle, opts)
		return true, newLease.LockHandle, nil
	}
}

func (locker *KubernetesLocker) release(lockHandle LockHandle) error {
	if err := locker.stopLeaseRenewWorker(lockHandle); err != nil {
		return err
	}

	return locker.changeLease(lockHandle, func(obj *unstructured.Unstructured, currentLease *LockLeaseRecord) error {
		locker.unsetObjectLockLease(obj, lockHandle.LockName)
		return nil
	})
}

func (locker *KubernetesLocker) stopLeaseRenewWorker(lockHandle LockHandle) error {
	locker.mux.Lock()
	defer locker.mux.Unlock()

	if doneChan, hasKey := locker.leaseRenewWorkers[lockHandle.ID]; !hasKey {
		return fmt.Errorf("unknown id %q for lock %q", lockHandle.ID, lockHandle.LockName)
	} else {
		doneChan <- struct{}{}
	}

	return nil
}

func (locker *KubernetesLocker) runLeaseRenewWorker(lockHandle LockHandle, opts AcquireOptions) {
	locker.mux.Lock()
	defer locker.mux.Unlock()

	locker.leaseRenewWorkers[lockHandle.ID] = make(chan struct{}, 0)
	go locker.extendLeaseRenewWorker(lockHandle, opts, locker.leaseRenewWorkers[lockHandle.ID])
}

func (locker *KubernetesLocker) extendLeaseRenewWorker(lockHandle LockHandle, opts AcquireOptions, doneChan chan struct{}) {
	ticker := time.NewTicker(lockLeaseRenewPeriodSeconds * time.Second)

	for {
		select {
		case <-ticker.C:
			if err := locker.extendLease(lockHandle); err == ErrLockAlreadyLeased || err == ErrNoExistingLockLeaseFound {
				fmt.Fprintf(os.Stderr, "ERROR: lost lease %s for lock %q\n", lockHandle.ID, lockHandle.LockName)
				if err := opts.OnLostLeaseFunc(lockHandle); err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
				}
				return
			} else if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: cannot extend lease %s for lock %q: %s\n", lockHandle.ID, lockHandle.LockName, err)
			}
		case <-doneChan:
		}
	}
}

func (locker *KubernetesLocker) extendLease(lockHandle LockHandle) error {
	return locker.changeLease(lockHandle, func(obj *unstructured.Unstructured, lease *LockLeaseRecord) error {
		lease.ExpiredAtTimestamp = time.Now().Unix() + lockLeaseTTLSeconds
		locker.setObjectLockLease(obj, lease)
		return nil
	})
}

func (locker *KubernetesLocker) changeLease(lockHandle LockHandle, changeFunc func(obj *unstructured.Unstructured, currentLease *LockLeaseRecord) error) error {
RETRY_CHANGE:

	if obj, err := locker.getResource(); err != nil {
		return err
	} else {
		debug("(change lock %q lease)  getResource -> %#v", lockHandle.LockName, obj)

		if currentLease, err := locker.extractObjectLockLease(obj, lockHandle.LockName); err != nil {
			return err
		} else if currentLease == nil {
			return ErrNoExistingLockLeaseFound
		} else if currentLease.ID != lockHandle.ID {
			return ErrLockAlreadyLeased
		} else {
			if err := changeFunc(obj, currentLease); err != nil {
				return err
			}
		}

		if _, err := locker.updateResource(obj); errors.IsConflict(err) {
			debug("(change lock %q lease)  update %#v conflict! Will retry...", lockHandle.LockName, obj)
			goto RETRY_CHANGE
		} else if err != nil {
			return err
		}

		return nil
	}
}

func debug(format string, args ...interface{}) {
	if os.Getenv("LOCKGATE_DEBUG") == "1" {
		fmt.Printf("LOCKGATE_DEBUG: %s\n", fmt.Sprintf(format, args...))
	}
}
