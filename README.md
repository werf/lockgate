# Lockgate

Lockgate is a locking library for go.

 - Classical interface with 2 modes of locking: shared and exclusive.
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

In the following example a `locker` object instance is created using either `NewFileLocker` or `NewKubernetesLocker` constructor — user should select needed locker implementation. The rest of the sample uses lockgate.Locker interface to acquire and release locks.

```
import "github.com/flant/lockgate"

func main() {
	// Create Kubernetes based locker in ns/mynamespace cm/myconfigmap.
	// Initialize kubeDynamicClient from https://github.com/kubernetes/client-go.
        locker := lockgate.NewKubernetesLocker(                                                          
                kubeDynamicClient, schema.GroupVersionResource{                                         
                        Group:    "",                                                                    
                        Version:  "v1",                                                                  
                        Resource: "configmaps",                                                          
                }, "myconfigmap", "mynamespace",                                                              
        )
	
	// OR create file based locker backed by /var/locks/mylocks_service_dir directory
    	locker, err := lockgate.NewFileLocker("/var/locks/mylocks_service_dir")
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to create file locker: %s\n", err)
		os.Exit(1)
	}

        // Case 1: simple blocking lock

	acquired, lock, err := locker.Acquire("myresource", lockgate.AcquireOptions{Shared: false, Timeout: 30*time.Second}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to lock myresource: %s\n", err)
		os.Exit(1)
	}

	// ...

	if err := locker.Release(lock); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
		os.Exit(1)
	}

	// Case 2: WithAcquire wrapper

	if err := lockgate.WithAcquire(locker, "myresource", lockgate.AcquireOptions{Shared: false, Timeout: 30*time.Second}, func(acquired bool) error {
		// ...
	}); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to perform an operation with locker myresource: %s\n", err)
		os.Exit(1)
	}
	
	// Case 3: non-blocking

	acquired, lock, err := locker.Acquire("myresource", lockgate.AcquireOptions{Shared: false, NonBlocking: true})
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to lock myresource: %s\n", err)
		os.Exit(1)
	}

	if acquired {
		// ...
	} else {
		// ...
	}

	if err := locker.Release(lock); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
		os.Exit(1)
	}
}
```
