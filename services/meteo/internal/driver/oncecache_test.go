package driver

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestOnceCacheFetchesEachKeyOnce checks that racing workers share a single fetch per key.
func TestOnceCacheFetchesEachKeyOnce(t *testing.T) {
	var cache onceCache[int]
	var calls atomic.Int64

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cache.get("shared", func() (int, error) {
				calls.Add(1)
				time.Sleep(5 * time.Millisecond)
				return 42, nil
			})
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != 42 {
				t.Errorf("expected 42, got %d", got)
			}
		}()
	}
	wg.Wait()

	if n := calls.Load(); n != 1 {
		t.Errorf("expected exactly 1 fetch for a shared key, got %d", n)
	}
}

// TestOnceCacheDoesNotSerializeDistinctKeys guards the property the drivers depend on: a slow
// lookup for one index must not block lookups for other indexes.
func TestOnceCacheDoesNotSerializeDistinctKeys(t *testing.T) {
	var cache onceCache[int]
	const workers = 8
	const fetchDelay = 50 * time.Millisecond

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _ = cache.get(string(rune('a'+i)), func() (int, error) {
				time.Sleep(fetchDelay)
				return i, nil
			})
		}(i)
	}
	wg.Wait()

	// Serialized fetches would take workers*fetchDelay; concurrent ones roughly one delay.
	if elapsed := time.Since(start); elapsed > fetchDelay*3 {
		t.Errorf("distinct keys appear serialized: %d fetches of %v took %v", workers, fetchDelay, elapsed)
	}
}

// TestOnceCacheRetriesAfterFailure verifies a failed fetch is not cached permanently.
func TestOnceCacheRetriesAfterFailure(t *testing.T) {
	var cache onceCache[int]
	var calls int

	fetch := func() (int, error) {
		calls++
		if calls == 1 {
			return 0, errors.New("transient upstream failure")
		}
		return 7, nil
	}

	if _, err := cache.get("key", fetch); err == nil {
		t.Fatal("expected first fetch to fail")
	}

	got, err := cache.get("key", fetch)
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if got != 7 {
		t.Errorf("expected 7 after retry, got %d", got)
	}
	if calls != 2 {
		t.Errorf("expected 2 fetch attempts, got %d", calls)
	}
}
