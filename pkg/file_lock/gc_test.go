package file_lock

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func writeLockFile(t *testing.T, dir, name string, age time.Duration) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	if age > 0 {
		mt := time.Now().Add(-age)
		if err := os.Chtimes(path, mt, mt); err != nil {
			t.Fatalf("chtimes %s: %v", path, err)
		}
	}
	return path
}

func TestGCLockFileDir_RemovesOldUnheld(t *testing.T) {
	dir := t.TempDir()
	old := writeLockFile(t, dir, "old", 48*time.Hour)

	if err := GCLockFileDir(dir, 24*time.Hour); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old unheld file must be removed, stat err=%v", err)
	}
}

func TestGCLockFileDir_KeepsFresh(t *testing.T) {
	dir := t.TempDir()
	fresh := writeLockFile(t, dir, "fresh", 0)

	if err := GCLockFileDir(dir, 24*time.Hour); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh file must be kept: %v", err)
	}
}

func TestGCLockFileDir_KeepsHeld(t *testing.T) {
	dir := t.TempDir()
	held := writeLockFile(t, dir, "held", 48*time.Hour)

	fl := flock.New(held)
	locked, err := fl.TryLock()
	if err != nil || !locked {
		t.Fatalf("setup lock failed: locked=%v err=%v", locked, err)
	}
	defer fl.Unlock()

	if err := GCLockFileDir(dir, 24*time.Hour); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if _, err := os.Stat(held); err != nil {
		t.Fatalf("held file must be kept: %v", err)
	}
}

func TestGCLockFileDir_Recurses(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	nested := writeLockFile(t, sub, "old", 48*time.Hour)

	if err := GCLockFileDir(dir, 24*time.Hour); err != nil {
		t.Fatalf("GC: %v", err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("nested old file must be removed, stat err=%v", err)
	}
}

func TestGCLockFileDir_MissingDir(t *testing.T) {
	if err := GCLockFileDir(filepath.Join(t.TempDir(), "nope"), time.Hour); err != nil {
		t.Fatalf("missing dir must be a no-op, got: %v", err)
	}
}

// TestAcquire_InodeSafeAfterGCUnlink simulates the flock+unlink race: a lock
// file is removed while "held" (as GC would after TryLock). A subsequent
// acquire must not silently return the dead inode — it must recreate the file
// so the lock file exists again afterwards.
func TestAcquire_InodeSafeAfterGCUnlink(t *testing.T) {
	dir := t.TempDir()
	lock := NewFileLock("some-name", dir).(*FileLock)
	path := lock.LockFilePath()

	// Pre-create then unlink the lock file to leave a stale name, mimicking a
	// GC pass that removed it just before we acquire.
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	locked, err := lock.TryLock(false)
	if err != nil {
		t.Fatalf("TryLock: %v", err)
	}
	if !locked {
		t.Fatal("expected to acquire lock")
	}
	defer lock.Unlock()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("acquire must leave a live lock file: %v", err)
	}
}
