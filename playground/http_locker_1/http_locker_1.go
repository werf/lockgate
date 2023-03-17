package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/werf/lockgate"
	"github.com/werf/lockgate/pkg/distributed_locker"
)

func run() error {
	backend := distributed_locker.NewHttpBackend("http://localhost:55589")
	l := distributed_locker.NewDistributedLocker(backend)

	acquired, lockHandle, err := l.Acquire("mylock", lockgate.AcquireOptions{
		OnWaitFunc: func(lockName string, doWait func() error) error {
			fmt.Printf("WAITING FOR %s\n", lockName)
			if err := doWait(); err != nil {
				fmt.Printf("WAITING FOR %s FAILED: %s\n", lockName, err)
				return err
			} else {
				fmt.Printf("WAITING FOR %s DONE\n", lockName)
			}
			return nil
		},
	})
	if err != nil {
		return err
	} else if acquired {
		fmt.Printf("acquired a lock: %#v\n", lockHandle)
	} else {
		fmt.Printf("not acquired lock %s\n", "mylock")
	}

	secs, err := strconv.Atoi(os.Args[1])
	if err != nil {
		return err
	}

	fmt.Printf("Sleeping %d seconds...\n", secs)

	time.Sleep(time.Duration(secs) * time.Second)

	return l.Release(lockHandle)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
}
