package util

import (
	"fmt"
	"os"
)

func Debug(format string, args ...interface{}) {
	if os.Getenv("LOCKGATE_DEBUG") == "1" {
		fmt.Printf("LOCKGATE_DEBUG: %s\n", fmt.Sprintf(format, args...))
	}
}
