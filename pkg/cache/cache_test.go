package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMemoryHitMissAndEviction(t *testing.T) {
	resultCache, err := NewMemory(time.Minute, 1)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	loader := func(context.Context) (any, error) {
		return calls.Add(1), nil
	}

	first, status, err := resultCache.GetOrLoad(context.Background(), "one", loader)
	if err != nil || status != ResultMiss || first.(int32) != 1 {
		t.Fatalf("first load = %v, %s, %v", first, status, err)
	}
	second, status, err := resultCache.GetOrLoad(context.Background(), "one", loader)
	if err != nil || status != ResultHit || second.(int32) != 1 {
		t.Fatalf("second load = %v, %s, %v", second, status, err)
	}

	if _, _, err := resultCache.GetOrLoad(context.Background(), "two", loader); err != nil {
		t.Fatal(err)
	}
	if resultCache.Len() != 1 {
		t.Fatalf("cache length = %d, want 1", resultCache.Len())
	}
	if _, status, err := resultCache.GetOrLoad(context.Background(), "one", loader); err != nil || status != ResultMiss {
		t.Fatalf("evicted key status = %s, error = %v", status, err)
	}
}

func TestMemoryExpiration(t *testing.T) {
	resultCache, err := NewMemory(15*time.Millisecond, 2)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	loader := func(context.Context) (any, error) {
		return calls.Add(1), nil
	}

	if _, _, err := resultCache.GetOrLoad(context.Background(), "key", loader); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, status, err := resultCache.GetOrLoad(context.Background(), "key", loader); err != nil || status != ResultMiss {
		t.Fatalf("expired key status = %s, error = %v", status, err)
	}
	if calls.Load() != 2 {
		t.Fatalf("loader calls = %d, want 2", calls.Load())
	}
}

func TestMemoryDeduplicatesConcurrentLoads(t *testing.T) {
	resultCache, err := NewMemory(time.Minute, 2)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	loader := func(context.Context) (any, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return "value", nil
	}

	const workers = 10
	var waitGroup sync.WaitGroup
	results := make(chan Result, workers)
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			_, status, loadErr := resultCache.GetOrLoad(context.Background(), "same", loader)
			if loadErr != nil {
				t.Errorf("GetOrLoad() error = %v", loadErr)
			}
			results <- status
		}()
	}
	<-started
	close(release)
	waitGroup.Wait()
	close(results)

	if calls.Load() != 1 {
		t.Fatalf("loader calls = %d, want 1", calls.Load())
	}
	misses := 0
	for status := range results {
		if status == ResultMiss {
			misses++
		}
	}
	if misses != 1 {
		t.Fatalf("misses = %d, want 1", misses)
	}
}
