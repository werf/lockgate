# Lockgate

Lockgate is a locking library for go.

 - Classical abstract interface with 2 modes of locking: shared and exclusive.
 - File locks on the single host are supported.
 - Kubernetes-based distributed locks are supported.
   - Kubernetes locker is configured by an arbitrary kubernetes resource.
   - Locks are stored in the annotations of the specified resource.
   - Properly use native kubernetes optimistic locking to handle simultaneous access to the resource.

# Installation

```
go get -u github.com/flant/lockgate
```

# Usage

```
import "github.com/flant/lockgate"

func main() {
    locker, err := lockgate.NewFileLocker("locks_service_dir")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to create file locker: %s\n", err)
		os.Exit(1)
	}

	// Case 1

	if err := locker.Acquire("myresource", lockgate.AcquireOptions{Shared: false, Timeout: 30*time.Second}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to lock myresource: %s\n", err)
		os.Exit(1)
	}

	// ...

	if err := locker.Release("myresource"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
		os.Exit(1)
	}

	// Case 2

	if err := lockgate.WithAcquire(locker, "myresource", lockgate.AcquireOptions{Shared: false, Timeout: 30*time.Second}, func(acquired bool) error {
		// ...
	}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to perform an operation with locker myresource: %s\n", err)
		os.Exit(1)
	}

	// Case 3

	acquired, err := locker.Acquire("myresource", lockgate.AcquireOptions{Shared: false, NonBlocking: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to lock myresource: %s\n", err)
		os.Exit(1)
	}

	if acquired {
		// ...
	} else {
		// ...
	}

	if err := locker.Release("myresource"); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
		os.Exit(1)
	}
}
```
