package optimistic_locking_store

import "sync"

type InMemoryStore struct {
	Mux    sync.Mutex
	Values map[string]*Value
}

type inMemoryRecordMetadata struct {
	Version int64
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{Values: make(map[string]*Value)}
}

func (store *InMemoryStore) GetValue(key string) (*Value, error) {
	store.Mux.Lock()
	defer store.Mux.Unlock()

	if rec, hasKey := store.Values[key]; hasKey {
		return rec, nil
	} else {
		rec := &Value{
			metadata: &inMemoryRecordMetadata{Version: 1},
		}
		store.Values[key] = rec

		return rec, nil
	}
}

func (store *InMemoryStore) PutValue(key string, value *Value) error {
	store.Mux.Lock()
	defer store.Mux.Unlock()

	valueMetadata := value.metadata.(*inMemoryRecordMetadata)

	if existingValue, hasKey := store.Values[key]; hasKey {
		existingValueMetadata := existingValue.metadata.(*inMemoryRecordMetadata)

		if existingValueMetadata.Version != valueMetadata.Version {
			return ErrRecordVersionChanged
		}
	}

	store.Values[key] = &Value{
		metadata: &inMemoryRecordMetadata{Version: valueMetadata.Version + 1},
		Data:     value.Data,
	}

	return nil
}
