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

	first, details, err := resultCache.GetOrLoad(context.Background(), "one", loader)
	if err != nil || details.Result != ResultMiss || first.(int32) != 1 {
		t.Fatalf("first load = %v, %+v, %v", first, details, err)
	}
	second, details, err := resultCache.GetOrLoad(context.Background(), "one", loader)
	if err != nil || details.Result != ResultHit || second.(int32) != 1 {
		t.Fatalf("second load = %v, %+v, %v", second, details, err)
	}

	if _, _, err := resultCache.GetOrLoad(context.Background(), "two", loader); err != nil {
		t.Fatal(err)
	}
	if resultCache.Len() != 1 {
		t.Fatalf("cache length = %d, want 1", resultCache.Len())
	}
	if _, details, err := resultCache.GetOrLoad(context.Background(), "one", loader); err != nil || details.Result != ResultMiss {
		t.Fatalf("evicted key details = %+v, error = %v", details, err)
	}
}

func TestMemoryReportsAgeTTLAndExpiration(t *testing.T) {
	const ttl = 15 * time.Minute
	resultCache, err := NewMemory(ttl, 2)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	resultCache.now = func() time.Time { return now }
	var calls atomic.Int32
	loader := func(context.Context) (any, error) {
		return calls.Add(1), nil
	}

	if _, details, err := resultCache.GetOrLoad(context.Background(), "key", loader); err != nil {
		t.Fatal(err)
	} else if details.Result != ResultMiss || details.Age != 0 || details.TTL != ttl {
		t.Fatalf("miss details = %+v", details)
	}
	now = now.Add(4*time.Minute + 30*time.Second)
	if _, details, err := resultCache.GetOrLoad(context.Background(), "key", loader); err != nil {
		t.Fatal(err)
	} else if details.Result != ResultHit || details.Age != 4*time.Minute+30*time.Second || details.TTL != ttl {
		t.Fatalf("hit details = %+v", details)
	}

	now = now.Add(11 * time.Minute)
	if _, details, err := resultCache.GetOrLoad(context.Background(), "key", loader); err != nil || details.Result != ResultMiss {
		t.Fatalf("expired key details = %+v, error = %v", details, err)
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
			_, details, loadErr := resultCache.GetOrLoad(context.Background(), "same", loader)
			if loadErr != nil {
				t.Errorf("GetOrLoad() error = %v", loadErr)
			}
			results <- details.Result
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

func TestDetailsMapsSharedToUserFacingHit(t *testing.T) {
	tests := []struct {
		result Result
		want   Result
	}{
		{result: ResultHit, want: ResultHit},
		{result: ResultMiss, want: ResultMiss},
		{result: ResultShared, want: ResultHit},
		{result: ResultBypass, want: ResultBypass},
		{result: Result("future"), want: ResultBypass},
	}

	for _, test := range tests {
		t.Run(string(test.result), func(t *testing.T) {
			if got := (Details{Result: test.result}).MetadataStatus(); got != test.want {
				t.Fatalf("MetadataStatus() = %q, want %q", got, test.want)
			}
		})
	}
}
