package driver

import "sync"

// onceCache memoizes per-key fetches for drivers that look up GRIB byte ranges through index
// files. Workers needing different keys never block each other, and workers racing for the same
// key perform a single fetch. Holding one mutex across the fetch instead would serialize the
// whole download pool behind every index lookup.
//
// Failed fetches are evicted so a later attempt can retry rather than inheriting the error.
type onceCache[T any] struct {
	mu      sync.Mutex
	entries map[string]*onceEntry[T]
}

type onceEntry[T any] struct {
	once  sync.Once
	value T
	err   error
}

// get returns the cached value for key, invoking fetch once if it is not present yet.
func (c *onceCache[T]) get(key string, fetch func() (T, error)) (T, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]*onceEntry[T])
	}
	entry, ok := c.entries[key]
	if !ok {
		entry = &onceEntry[T]{}
		c.entries[key] = entry
	}
	c.mu.Unlock()

	entry.once.Do(func() {
		entry.value, entry.err = fetch()
		if entry.err != nil {
			c.mu.Lock()
			delete(c.entries, key)
			c.mu.Unlock()
		}
	})

	return entry.value, entry.err
}
