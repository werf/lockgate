package util

import (
	"encoding/json"
	"errors"
	"fmt"
)

type SerializableError struct {
	Error error
}

func (obj SerializableError) String() string {
	if obj.Error != nil {
		return obj.Error.Error()
	} else {
		return fmt.Sprintf("%v", nil)
	}
}

func (obj SerializableError) MarshalJSON() ([]byte, error) {
	var errMsg string
	if obj.Error != nil {
		errMsg = obj.Error.Error()
	}
	return json.Marshal(errMsg)
}

func (obj *SerializableError) UnmarshalJSON(data []byte) error {
	var errMsg string
	if err := json.Unmarshal(data, &errMsg); err != nil {
		return err
	}
	if errMsg != "" {
		obj.Error = errors.New(errMsg)
	}
	return nil
}
