package optimistic_locking_store

import "errors"

var (
	ErrRecordVersionChanged = errors.New("record version changed")
)

type OptimisticLockingStore interface {
	GetValue(key string) (*Value, error)
	PutValue(key string, value *Value) error
}

type Value struct {
	Data     string
	metadata interface{}
}

func IsErrRecordVersionChanged(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == ErrRecordVersionChanged.Error()
}
