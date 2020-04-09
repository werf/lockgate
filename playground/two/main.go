package main

import (
	"fmt"
	"os"
	"time"

	"github.com/flant/lockgate"

	"github.com/flant/kubedog/pkg/kube"
)

func do() error {
	if err := kube.Init(kube.InitOptions{}); err != nil {
		return fmt.Errorf("cannot initialize kube: %s", err)
	}

	if locker, err := lockgate.NewFileLocker("/tmp/locks"); err != nil {
		return fmt.Errorf("init error: %s", err)
	} else {
		if _, err := locker.Acquire("mylock", lockgate.AcquireOptions{
			OnWaitFunc: func(_ string, doWait func() error) error {
				fmt.Printf("WAITING!\n")
				defer fmt.Printf("DONE!")
				return doWait()
			},
		}); err != nil {
			return fmt.Errorf("acquire mylock error: %s", err)
		}
		defer locker.Release("mylock")
	}

	fmt.Printf("ACQUIRED!\n")
	time.Sleep(10 * time.Second)

	return nil
}

func main() {
	if err := do(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("GOOD.\n")
}
