package lockgate

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/kubelock"
	"github.com/google/uuid"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type KubernetesLocker struct {
	KubernetesInterface dynamic.Interface
	GVR                 schema.GroupVersionResource
	ResourceName        string
	Namespace           string

	kubeLock   *kubelock.KubeLock
	lockLeases map[string]*lockLease
}

type lockLease struct {
	UUID       uuid.UUID
	lockObject kubelock.NamespaceableLock
}

func NewKubernetesLocker(kubernetesInterface dynamic.Interface, gvr schema.GroupVersionResource, resourceName string, namespace string) (*KubernetesLocker, error) {
	if kubeLock, err := kubelock.New(kubelock.Config{DynClient: kubernetesInterface, GVR: gvr}); err != nil {
		return nil, err
	} else {
		return &KubernetesLocker{
			KubernetesInterface: kubernetesInterface,
			GVR:                 gvr,
			ResourceName:        resourceName,
			Namespace:           namespace,
			kubeLock:            kubeLock,
			lockLeases:          make(map[string]*lockLease),
		}, nil
	}
}

func (locker *KubernetesLocker) Acquire(name string, opts AcquireOptions) (bool, error) {
	if _, hasKey := locker.lockLeases[name]; hasKey {
		panic(fmt.Sprintf("%q already locked: multiple acquires in the single process are not allowed!", name))
	}

	lockLease := &lockLease{UUID: uuid.New(), lockObject: locker.kubeLock.Lock(name)}
	locker.lockLeases[name] = lockLease

	if locker.Namespace != "" {
		return true, lockLease.lockObject.Namespace(locker.Namespace).Acquire(context.Background(), locker.ResourceName, kubelock.AcquireOptions{Owner: lockLease.UUID.String(), TTL: 10 * time.Second})
	} else {
		return true, lockLease.lockObject.Acquire(context.Background(), locker.ResourceName, kubelock.AcquireOptions{Owner: lockLease.UUID.String(), TTL: 10 * time.Second})
	}
}

func (locker *KubernetesLocker) Release(name string) error {
	if lockLease, hasKey := locker.lockLeases[name]; !hasKey {
		panic(fmt.Sprintf("%q is not locked", name))
	} else {
		if locker.Namespace != "" {
			return lockLease.lockObject.Namespace(locker.Namespace).Release(context.Background(), locker.ResourceName, kubelock.ReleaseOptions{Owner: lockLease.UUID.String()})
		} else {
			return lockLease.lockObject.Release(context.Background(), locker.ResourceName, kubelock.ReleaseOptions{Owner: lockLease.UUID.String()})
		}
	}
}
