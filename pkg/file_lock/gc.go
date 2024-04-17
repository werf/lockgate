package file_lock

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

func GCLockFileDir(dirPath string) error {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return fmt.Errorf("error reading directory: %v", err)
	}

	for _, entry := range files {
		if entry.IsDir() {
			if err := GCLockFileDir(filepath.Join(dirPath, entry.Name())); err != nil {
				return fmt.Errorf("error processing directory %s: %v", entry.Name(), err)
			}
			continue
		}

		filePath := filepath.Join(dirPath, entry.Name())
		err := safeDeleteLockFile(filePath)
		if err != nil {
			return fmt.Errorf("error deleting file %s: %v", filePath, err)
		}
	}

	return nil
}

func safeDeleteLockFile(filePath string) error {
	fileLock := flock.New(filePath)
	locked, err := fileLock.TryLock()
	if err != nil {
		return fmt.Errorf("error trying to lock file %s: %v", filePath, err)
	}

	// If the file is already locked, we shouldn't delete it.
	if !locked {
		return nil
	}
	defer fileLock.Unlock()

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("error removing file %s: %v", filePath, err)
	}

	return nil
}
