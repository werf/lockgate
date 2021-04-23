# Lockgate

Lockgate is a locking library for Go.

 - Classical interface:
   - 2 types of locks: shared and exclusive;
   - 2 modes of locking: blocking and non-blocking.
 - **File locks** on the single host are supported.
 - **Kubernetes-based distributed locks** are supported:
   - Kubernetes locker is configured by an arbitrary Kubernetes resource;
   - locks are stored in the annotations of the specified resource;
   - properly use native Kubernetes optimistic locking to handle simultaneous access to the resource.
 - **Locks using HTTP server** are supported:
   - lockgate lock server may be run as a standalone process or multiple Kubernetes-backed processes:
      - lockgate locks server uses in-memory or Kubernetes key-value storage with optimistic locks;
   - user specifies URL of a lock server instance in the userspace code to use locks over HTTP.

This library is used in the [werf CI/CD tool](https://github.com/werf/werf) to implement synchronization of multiple werf build and deploy processes running from single or multiple hosts using Kubernetes or local file locks.

If you have an Open Source project using lockgate, feel free to list it here via PR.

- [Installation](#installation)
- [Usage](#usage)
  - [Select a locker](#select-a-locker)
    - [File locker](#file-locker)
    - [Kubernetes locker](#kubernetes-locker)
    - [HTTP locker](#http-locker)
  - [Lockgate HTTP lock server](#lockgate-http-lock-server)
  - [Locker usage example](#locker-usage-example)
- [Feedback](#feedback)


# Installation

```
go get -u github.com/werf/lockgate
```

# Usage

## Select a locker

The main interface of the library which user interacts with is `lockgate.Locker`. There are multiple implementations of the locker available: 

* [file locker](#file-locker),
* [Kubernetes locker](#kubernetes-locker),
* [HTTP locker](#http-locker).

### File locker

This is a simple OS filesystem locks based locker. It can be used by multiple processes on the single host filesystem.

Create a file locker as follows:

```
import "github.com/werf/lockgate"

...

locker, err := lockgate.NewFileLocker("/var/lock/myapp")
```

All cooperating processes should use the same locks directory.

### Kubernetes locker

This locker uses specified Kubernetes resource as a storage for locker data. Multiple processes which use this locker should have an access to the same Kubernetes cluster.

This locker allows distributed locking over multiple hosts.

Create a Kubernetes locker as follows:

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

All cooperating processes should use the same Kubernetes params. In this example, locks data will be stored in the `mycm` ConfigMap in the `myns` namespace.

### HTTP locker

This locker uses lockgate HTTP server to organize locks and allows distributed locking over multiple hosts.

Create a HTTP locker as follows:

```
import "github.com/werf/lockgate"

...

locker := lockgate.NewHttpLocker("http://localhost:55589")
```

All cooperating processes should use the same URL endpoint of the lockgate HTTP lock server. In this example, there should be a lockgate HTTP lock server available at `localhost:55589` address. See below how to run such a server.

## Lockgate HTTP lock server

Lockgate HTTP server can use memory-storage or kubernetes-storage:

- There can be only 1 instance of the lockgate server that uses memory storage.
- There can be an arbitrary number of servers using kubernetes-storage.

Run a lockgate HTTP lock server as follows:

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

## Locker usage example

In the following example, a `locker` object instance is created using one of the ways documented above — user should select the required locker implementation. The rest of the sample uses generic `lockgate.Locker` interface to acquire and release locks.

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

# Community

Please feel free to reach us via [project's Discussions](https://github.com/werf/lockgate/discussions) and [werf's Telegram group](https://t.me/werf_io) (there's [another one in Russian](https://t.me/werf_ru) as well).

You're also welcome to follow [@werf_io](https://twitter.com/werf_io) to stay informed about all important news, articles, etc.
