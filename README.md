# Lockgate

Lockgate is a locking library for go.

 - Classical interface:
   - 2 types of locks: shared and exclusive;
   - 2 modes of locking: blocking and non-blocking.
 - File locks on the single host are supported.
 - Kubernetes-based distributed locks are supported:
   - kubernetes locker is configured by an arbitrary kubernetes resource;
   - locks are stored in the annotations of the specified resource;
   - properly use native kubernetes optimistic locking to handle simultaneous access to the resource.
 - Locks using http lockgate locks server are supported:
   - lockgate locks server may be run as a standalone process or multiple kubernetes-backed processes:
      - lockgate locks server uses in-memory or kubernetes key-value storage with optimistic-locks;
   - user specifies URL of lock server instance in the userspace code to use locks over http;

This library is used in the [werf CI/CD tool](https://github.com/werf/werf) to implement synchronization of multiple werf build and deploy processes running from single or multiple hosts using Kubernetes or local file locks.

If you have an Open Source project using lockgate, feel free to list it here.

# Installation

```
go get -u github.com/werf/lockgate
```

# Usage


## Select a locker

The main interface of the library which user interacts with is `lockgate.Locker`. There are multiple implementations of locker available: file locker, kubernetes locker and http locker.

### File locker

This is a simple OS filesystem locks based locker, which can be used by multiple processes on the single host filesystem.

Create a file locker as follows:

```
import "github.com/werf/lockgate"

...

locker, err := lockgate.NewFileLocker("/var/lock/myapp")
```

All cooperating processes should use the same locks directory.

### Kubernetes locker

This locker uses specified kubernetes resource as a storage for locker data. Multiple processes which use this locker should have an access to the same kubernetes cluster.

This locker allows distributed locking over multiple hosts.

Create kubernetes locker as follows:

```
import "github.com/werf/lockgate"

...

// Initialize kubeDynamicClient from https://github.com/kubernetes/client-go.
locker, err := lockgate.NewKubernetesLocker(
	kubeDynamicClient, schema.GroupVersionResource{
		Group:    "",
		Version:  "v1",
		Resource: "configmaps",
	}, "mycm", "myns",
)
```

All cooperating processes should use the same kubernetes-params. In this example locks data will be stored in the Namesapce "myns" ConfigMap "mycm".

### Http locker

This locker uses lockgate http server to organize locks and allows distributed locking over multiple hosts.

Create http locker as follows:

```
import "github.com/werf/lockgate"

...

locker := lockgate.NewHttpLocker("http://localhost:55589")
```

All cooperating processes should use the same kubernetes-params. In this example there should be lockgate http locker server avaiable at `localhost:55589` address. See below how to bring up such server.

## Lockgate http locker server

Lockgate http server can use memory-storage or kubernetes-storage. There can be only 1 instance of lockgate server, that uses memory storage and there can be arbitrary number of servers that use kubernetes-storage.

Run lockgate http locker server as follows:

```
import "github.com/werf/lockgate"
import "github.com/werf/lockgate/pkg/distributed_locker"
import "github.com/werf/lockgate/pkg/distributed_locker/optimistic_locking_store"

...
store := optimistic_locking_store.NewInMemoryStore()
// OR
// store := optimistic_locking_store.NewKubernetesResourceAnnotationsStore(
//	kube.DynamicClient, schema.GroupVersionResource{
//		Group:    "",
//		Version:  "v1",
//		Resource: "configmaps",
//	}, "mycm", "myns",
//)
backend := distributed_locker.NewOptimisticLockingStorageBasedBackend(store)
distributed_locker.RunHttpBackendServer("0.0.0.0", "55589", backend)
```

## Locker usage

In the following example a `locker` object instance is created using one of the ways documented above — user should select needed locker implementation. The rest of the sample uses generic lockgate.Locker interface to acquire and release locks.

```
import "github.com/werf/lockgate"

func main() {
	// Create Kubernetes based locker in ns/mynamespace cm/myconfigmap.
	// Initialize kubeDynamicClient using https://github.com/kubernetes/client-go.
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

		if err := locker.Release(lock); err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to unlock myresource: %s\n", err)
			os.Exit(1)
		}
	} else {
		// ...
	}
}
```
