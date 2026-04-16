package cache

import (
	"sync"
	"time"
)

// MemoryCache 简单的内存缓存
type MemoryCache struct {
	data      map[string]interface{}
	expires   map[string]time.Time
	mu        sync.RWMutex
	defaultTTL time.Duration
	stopCh    chan struct{}
}

// New 创建新缓存
func New(defaultTTL time.Duration) *MemoryCache {
	c := &MemoryCache{
		data:      make(map[string]interface{}),
		expires:   make(map[string]time.Time),
		defaultTTL: defaultTTL,
		stopCh:    make(chan struct{}),
	}
	// 启动过期清理
	go c.cleanup()
	return c
}

// Stop 停止清理goroutine
func (c *MemoryCache) Stop() {
	close(c.stopCh)
}

// Get 获取缓存
func (c *MemoryCache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if exp, ok := c.expires[key]; ok && time.Now().After(exp) {
		return nil, false
	}

	if data, ok := c.data[key]; ok {
		return data, true
	}
	return nil, false
}

// Set 设置缓存
func (c *MemoryCache) Set(key string, data interface{}) {
	c.SetWithTTL(key, data, c.defaultTTL)
}

// SetWithTTL 设置带过期时间的缓存
func (c *MemoryCache) SetWithTTL(key string, data interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = data
	c.expires[key] = time.Now().Add(ttl)
}

// Delete 删除缓存
func (c *MemoryCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	delete(c.expires, key)
}

// Clear 清空所有缓存
func (c *MemoryCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = make(map[string]interface{})
	c.expires = make(map[string]time.Time)
}

// cleanup 定期清理过期项
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for key, exp := range c.expires {
				if now.After(exp) {
					delete(c.data, key)
					delete(c.expires, key)
				}
			}
			c.mu.Unlock()
		case <-c.stopCh:
			return
		}
	}
}
