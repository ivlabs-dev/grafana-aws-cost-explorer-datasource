package cache

import (
	"container/list"
	"context"
	"fmt"
	"sync"
	"time"
)

type Result string

const (
	ResultHit    Result = "hit"
	ResultMiss   Result = "miss"
	ResultShared Result = "shared"
)

type Loader func(context.Context) (any, error)

// Cache allows the initial in-memory implementation to be replaced by a
// persistent or distributed implementation without changing query execution.
type Cache interface {
	GetOrLoad(context.Context, string, Loader) (any, Result, error)
	Len() int
}

type entry struct {
	key       string
	value     any
	expiresAt time.Time
}

type flight struct {
	done  chan struct{}
	value any
	err   error
}

// Memory is a bounded TTL cache with LRU eviction and per-key in-flight request
// deduplication. Failed loader calls are never cached.
type Memory struct {
	mu         sync.Mutex
	ttl        time.Duration
	maxEntries int
	entries    map[string]*list.Element
	lru        *list.List
	flights    map[string]*flight
	now        func() time.Time
}

func NewMemory(ttl time.Duration, maxEntries int) (*Memory, error) {
	if ttl <= 0 {
		return nil, fmt.Errorf("cache TTL must be greater than zero")
	}
	if maxEntries <= 0 {
		return nil, fmt.Errorf("cache maximum entries must be greater than zero")
	}

	return &Memory{
		ttl:        ttl,
		maxEntries: maxEntries,
		entries:    make(map[string]*list.Element, maxEntries),
		lru:        list.New(),
		flights:    make(map[string]*flight),
		now:        time.Now,
	}, nil
}

func (c *Memory) GetOrLoad(ctx context.Context, key string, loader Loader) (any, Result, error) {
	if key == "" {
		return nil, ResultMiss, fmt.Errorf("cache key must not be empty")
	}
	if loader == nil {
		return nil, ResultMiss, fmt.Errorf("cache loader must not be nil")
	}

	c.mu.Lock()
	if element, ok := c.entries[key]; ok {
		item := element.Value.(*entry)
		if c.now().Before(item.expiresAt) {
			c.lru.MoveToFront(element)
			value := item.value
			c.mu.Unlock()
			return value, ResultHit, nil
		}
		c.removeElement(element)
	}

	if pending, ok := c.flights[key]; ok {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ResultShared, ctx.Err()
		case <-pending.done:
			return pending.value, ResultShared, pending.err
		}
	}

	pending := &flight{done: make(chan struct{})}
	c.flights[key] = pending
	c.mu.Unlock()

	value, err := loader(ctx)

	c.mu.Lock()
	pending.value = value
	pending.err = err
	if err == nil {
		c.insert(key, value)
	}
	delete(c.flights, key)
	close(pending.done)
	c.mu.Unlock()

	return value, ResultMiss, err
}

func (c *Memory) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeExpired()
	return len(c.entries)
}

func (c *Memory) insert(key string, value any) {
	if existing, ok := c.entries[key]; ok {
		item := existing.Value.(*entry)
		item.value = value
		item.expiresAt = c.now().Add(c.ttl)
		c.lru.MoveToFront(existing)
		return
	}

	element := c.lru.PushFront(&entry{
		key:       key,
		value:     value,
		expiresAt: c.now().Add(c.ttl),
	})
	c.entries[key] = element

	for len(c.entries) > c.maxEntries {
		c.removeElement(c.lru.Back())
	}
}

func (c *Memory) removeExpired() {
	now := c.now()
	for key, element := range c.entries {
		if !now.Before(element.Value.(*entry).expiresAt) {
			c.removeElement(element)
			delete(c.entries, key)
		}
	}
}

func (c *Memory) removeElement(element *list.Element) {
	if element == nil {
		return
	}
	c.lru.Remove(element)
	delete(c.entries, element.Value.(*entry).key)
}
