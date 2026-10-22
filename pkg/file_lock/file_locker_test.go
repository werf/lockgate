package file_lock

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

// TestPollLock_TimeoutBoundaryDoesNotDeadlock reproduces the race between the
// poll goroutine acquiring the flock and the Lock timeout firing: with
// unbuffered channels the polling goroutine blocked sending the acquire result
// while pollLock blocked sending the cancellation, and neither side ever read.
//
// Each waiter starts on a held lock that is released before the first poll
// tick; the timeouts are spread by microseconds around that tick so that for
// some of the waiters both events land in the same instant.
func TestPollLock_TimeoutBoundaryDoesNotDeadlock(t *testing.T) {
	const (
		rounds       = 2
		waiters      = 40
		releaseAfter = 150 * time.Millisecond
		pollTick     = 500 * time.Millisecond
		timeoutStep  = 30 * time.Microsecond
	)

	dir := t.TempDir()

	for round := 0; round < rounds; round++ {
		var wg sync.WaitGroup

		for w := 0; w < waiters; w++ {
			holder := NewFileLock(fmt.Sprintf("timeout-boundary-%d-%d", round, w), dir)
			locked, err := holder.TryLock(false)
			if err != nil {
				t.Fatalf("setup TryLock %s: %v", holder.GetName(), err)
			}
			if !locked {
				t.Fatalf("setup lock %s not acquired", holder.GetName())
			}

			released := make(chan error, 1)
			time.AfterFunc(releaseAfter, func() {
				released <- holder.Unlock()
			})

			waiter := NewFileLock(holder.GetName(), dir).(*FileLock)
			timeout := pollTick + time.Duration(w)*timeoutStep
			done := make(chan error, 1)
			go func() {
				done <- waiter.Lock(timeout, false, nil)
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()

				select {
				case err := <-done:
					if err == nil {
						if err := waiter.Unlock(); err != nil {
							t.Errorf("Unlock %s: %v", waiter.GetName(), err)
							return
						}
					}
				case <-time.After(timeout + 5*time.Second):
					t.Errorf("Lock %s did not return: pollLock deadlocked at the timeout boundary", waiter.GetName())
					return
				}

				if err := <-released; err != nil {
					t.Errorf("releasing held lock %s: %v", waiter.GetName(), err)
					return
				}

				// On timeout the poll goroutine may still have acquired the
				// flock in the very last moment: it must not be left behind.
				fl := flock.New(waiter.LockFilePath())
				free, err := fl.TryLock()
				if err != nil {
					t.Errorf("TryLock %s after Lock returned: %v", waiter.GetName(), err)
					return
				}
				if !free {
					t.Errorf("flock of %s is still held after Lock returned", waiter.GetName())
					return
				}
				if err := fl.Unlock(); err != nil {
					t.Errorf("Unlock %s: %v", waiter.GetName(), err)
				}
			}()
		}

		wg.Wait()

		if t.Failed() {
			t.FailNow()
		}
	}
}
