package api

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockEngineSemaphore_LimitsConcurrency(t *testing.T) {
	var api Api
	api.lockEngineSem = make(chan struct{}, lockEngineConcurrency)

	var current int64
	var maxObserved int64
	var wg sync.WaitGroup

	// Launch double the concurrency limit to force contention
	total := lockEngineConcurrency * 2
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			api.acquireLockSem()
			defer api.releaseLockSem()

			cur := atomic.AddInt64(&current, 1)
			for {
				max := atomic.LoadInt64(&maxObserved)
				if cur <= max || atomic.CompareAndSwapInt64(&maxObserved, max, cur) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt64(&current, -1)
		}()
	}
	wg.Wait()

	if maxObserved > int64(lockEngineConcurrency) {
		t.Fatalf("concurrency exceeded semaphore capacity: max=%d capacity=%d",
			maxObserved, lockEngineConcurrency)
	}
}

func TestLockEngineSemaphore_NilSemaphoreIsNoop(t *testing.T) {
	var api Api
	// lockEngineSem is nil — acquire/release should not block or panic
	api.acquireLockSem()
	api.releaseLockSem()
}

func TestLockEngineSemaphore_ReleasesAfterCompletion(t *testing.T) {
	var api Api
	api.lockEngineSem = make(chan struct{}, lockEngineConcurrency)

	// Fill the semaphore
	for i := 0; i < lockEngineConcurrency; i++ {
		api.acquireLockSem()
	}

	// Now the semaphore is full. Release one slot and verify it can be reacquired.
	api.releaseLockSem()

	done := make(chan struct{})
	go func() {
		api.acquireLockSem()
		close(done)
	}()

	select {
	case <-done:
		// success: slot was available after release
	case <-time.After(time.Second):
		t.Fatal("semaphore did not release a slot after releaseLockSem")
	}
}
