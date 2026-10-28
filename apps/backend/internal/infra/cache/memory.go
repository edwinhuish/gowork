package cache

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// memoryCache 是进程内缓存，仅用于本地开发或 Redis 不可用时的降级。
// 它不具备跨实例共享能力，重启即失效。
type memoryCache struct {
	mu    sync.Mutex
	items map[string]*memEntry
	stop  chan struct{}
	once  sync.Once
}

type memEntry struct {
	value    string
	expireAt time.Time
}

func newMemory() *memoryCache {
	c := &memoryCache{
		items: make(map[string]*memEntry),
		stop:  make(chan struct{}),
	}
	go c.sweep()
	return c
}

func (c *memoryCache) Get(_ context.Context, key string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		return "", ErrMiss
	}
	if !e.expireAt.IsZero() && time.Now().After(e.expireAt) {
		delete(c.items, key)
		return "", ErrMiss
	}
	return e.value, nil
}

func (c *memoryCache) Set(_ context.Context, key, value string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	e := &memEntry{value: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	}
	c.items[key] = e
	return nil
}

func (c *memoryCache) Del(_ context.Context, keys ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, k := range keys {
		delete(c.items, k)
	}
	return nil
}

func (c *memoryCache) Incr(_ context.Context, key string, ttl time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	e, ok := c.items[key]
	if !ok || (!e.expireAt.IsZero() && now.After(e.expireAt)) {
		e = &memEntry{value: "0"}
		if ttl > 0 {
			e.expireAt = now.Add(ttl)
		}
		c.items[key] = e
	}

	n, err := strconv.ParseInt(e.value, 10, 64)
	if err != nil {
		n = 0
	}
	n++
	e.value = strconv.FormatInt(n, 10)
	return n, nil
}

func (c *memoryCache) Ping(context.Context) error { return nil }

func (c *memoryCache) Driver() string { return "memory" }

func (c *memoryCache) Close() error {
	c.once.Do(func() { close(c.stop) })
	return nil
}

// sweep 定期清理过期键，避免长期运行时内存无界增长。
func (c *memoryCache) sweep() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.stop:
			return
		case now := <-ticker.C:
			c.mu.Lock()
			for k, e := range c.items {
				if !e.expireAt.IsZero() && now.After(e.expireAt) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		}
	}
}
