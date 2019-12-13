# Shluz

Shluz is a locking library for go.

Currently only file locks are supported, processes that use locks should work on a single host and use the same locks directory (see `shluz.Init` function).

# Installation

```
go get -u github.com/flant/shluz
```

# Usage

```
import "github.com/flant/shluz"

func main() {
	if err := shluz.Init("locks_service_dir"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to init shluz: %s\n", err)
		os.Exit(1)
	}

	// Case 1

	if err := Shluz.Lock("myresource", Shluz.LockOptions{ReadOnly: false, Timeout: 30*time.Second}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to lock myresource: %s\n", err)
		os.Exit(1)
	}

	// ...

	if err := Shluz.Unlock("myresource"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
		os.Exit(1)
	}

	// Case 2

	if err := Shluz.WithLock("myresource", Shluz.LockOptions{ReadOnly: false, Timeout: 30*time.Second}, func() error {
		// ...
	}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to perform action with locked myresource: %s\n", err)
		os.Exit(1)
	}

	// Case 3

	locked, err := Shluz.TryLock("myresource", Shluz.TryLockOptions{ReadOnly: false})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to lock myresource: %s\n", err)
		os.Exit(1)
	}

	if locked {
		// ...
	} else {
		// ...
	}

	if err := Shluz.Unlock("myresource"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
		os.Exit(1)
	}
}
```
