package main

import (
	"fmt"
	"os"

	"github.com/werf/lockgate/pkg/distributed_locker"
	"github.com/werf/lockgate/pkg/distributed_locker/optimistic_locking_store"
)

func run() error {
	store := optimistic_locking_store.NewInMemoryStore()
	backend := distributed_locker.NewOptimisticLockingStorageBasedBackend(store)
	return distributed_locker.RunHttpBackendServer("0.0.0.0", "55589", backend)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}
