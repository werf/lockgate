package main

import (
	"fmt"
	"os"

	"github.com/flant/lockgate"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/flant/kubedog/pkg/kube"
)

func do() error {
	if err := kube.Init(kube.InitOptions{}); err != nil {
		return fmt.Errorf("cannot initialize kube: %s", err)
	}

	if locker, err := lockgate.NewKubernetesLocker(
		kube.DynamicClient, schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "configmaps",
		}, "mycm", "myns",
	); err != nil {
		return fmt.Errorf("init error: %s", err)
	} else {
		if _, err := locker.Acquire("mylock", lockgate.AcquireOptions{}); err != nil {
			return fmt.Errorf("acquire mylock error: %s", err)
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
