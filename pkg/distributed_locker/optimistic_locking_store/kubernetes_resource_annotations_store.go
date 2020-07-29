package optimistic_locking_store

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type KubernetesResourceAnnotationsStore struct {
	KubernetesInterface dynamic.Interface
	GVR                 schema.GroupVersionResource
	ResourceName        string
	Namespace           string
}

func NewKubernetesResourceAnnotationsStore(kubernetesInterface dynamic.Interface, gvr schema.GroupVersionResource, resourceName string, namespace string) *KubernetesResourceAnnotationsStore {
	return &KubernetesResourceAnnotationsStore{
		KubernetesInterface: kubernetesInterface,
		GVR:                 gvr,
		ResourceName:        resourceName,
		Namespace:           namespace,
	}
}

func (store *KubernetesResourceAnnotationsStore) GetValue(key string) (*Value, error) {
	debug("KubernetesResourceAnnotationsStore.GetValue by key %q", key)

	if obj, err := store.getResource(); err != nil {
		return nil, err
	} else {
		value := &Value{
			Data:     obj.GetAnnotations()[key],
			metadata: obj,
		}

		debug("KubernetesResourceAnnotationsStore.GetValue by key %q -> %#v", key, value)

		return value, nil
	}
}

func (store *KubernetesResourceAnnotationsStore) PutValue(key string, value *Value) error {
	debug("KubernetesResourceAnnotationsStore.PutValue %s %#v", key, value)

	obj := value.metadata.(*unstructured.Unstructured)

	annots := obj.GetAnnotations()

	if value.Data != "" {
		if annots == nil {
			annots = make(map[string]string)
		}
		annots[key] = value.Data
		obj.SetAnnotations(annots)
	} else if annots != nil {
		delete(annots, key)
		obj.SetAnnotations(annots)
	}

	debug("KubernetesResourceAnnotationsStore.PutValue annots=%#v", obj.GetAnnotations())

	if _, err := store.updateResource(obj); isOptimisticLockingError(err) {
		return ErrRecordVersionChanged
	} else if err != nil {
		return err
	}
	return nil
}

func (store *KubernetesResourceAnnotationsStore) getResource() (*unstructured.Unstructured, error) {
	var err error
	var obj *unstructured.Unstructured

	if store.Namespace == "" {
		obj, err = store.KubernetesInterface.Resource(store.GVR).Get(context.Background(), store.ResourceName, metav1.GetOptions{})
	} else {
		obj, err = store.KubernetesInterface.Resource(store.GVR).Namespace(store.Namespace).Get(context.Background(), store.ResourceName, metav1.GetOptions{})
	}

	if err != nil {
		return obj, fmt.Errorf("cannot get %s by name %q: %s", store.GVR.String(), store.ResourceName, err)
	}
	return obj, err
}

func (store *KubernetesResourceAnnotationsStore) updateResource(obj *unstructured.Unstructured) (*unstructured.Unstructured, error) {
	var err error
	var newObj *unstructured.Unstructured

	if store.Namespace == "" {
		newObj, err = store.KubernetesInterface.Resource(store.GVR).Update(context.Background(), obj, metav1.UpdateOptions{})
	} else {
		newObj, err = store.KubernetesInterface.Resource(store.GVR).Namespace(store.Namespace).Update(context.Background(), obj, metav1.UpdateOptions{})
	}

	if errors.IsAlreadyExists(err) || err == nil {
		return newObj, err
	} else {
		return newObj, fmt.Errorf("cannot update %s by name %s: %s", store.GVR.String(), store.ResourceName, err)
	}
}

func isOptimisticLockingError(err error) bool {
	if err != nil {
		return strings.HasSuffix(err.Error(), "the object has been modified; please apply your changes to the latest version and try again")
	}
	return false
}
