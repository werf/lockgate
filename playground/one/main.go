package main

import (
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/werf/kubedog/pkg/kube"
	"github.com/werf/lockgate"
)

func do() error {
	if err := kube.Init(kube.InitOptions{}); err != nil {
		return fmt.Errorf("cannot initialize kube: %s", err)
	}

	locker := lockgate.NewKubernetesLocker(
		kube.DynamicClient, schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "configmaps",
		}, "mycm", "myns",
	)

	if _, lock, err := locker.Acquire("mylock", lockgate.AcquireOptions{}); err != nil {
		return fmt.Errorf("acquire mylock error: %s", err)
	} else {
		fmt.Printf("ACQUIRED %#v!\n", lock)
		for i := 0; i < 60; i++ {
			time.Sleep(1 * time.Second)
			fmt.Printf("%d\n", i)
		}
		if err := locker.Release(lock); err != nil {
			return fmt.Errorf("release mylock error: %s", err)
		}
	}

	return nil
}

func main() {
	if err := do(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("GOOD.\n")
}
