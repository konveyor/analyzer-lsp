package base

import (
	"sync"
)

// AwaitCache is a generic cache that allows values to be awaited until they are
// ready.
type AwaitCache[K comparable, V any] struct {
	// A map to store values associated with keys
	cache map[K]*AwaitCacheValue[V]

	// A read-write mutex to protect concurrent access
	mu sync.RWMutex
}

// NewAwaitCache creates and returns a new AwaitCache instance.
func NewAwaitCache[K comparable, V any]() *AwaitCache[K, V] {
	return &AwaitCache[K, V]{
		cache: make(map[K]*AwaitCacheValue[V]),
	}
}

// Get retrieves a value associated with a key. If the value doesn't exist, it
// creates a new one.
//
// TODO(jsussman): Make this use RLock and RUnlock somehow
func (ac *AwaitCache[K, V]) Get(key K) *AwaitCacheValue[V] {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.cache[key] == nil {
		ac.cache[key] = NewAwaitCacheValue[V]()
	}
	return ac.cache[key]
}

// Set sets a value associated with a key in the cache.
func (ac *AwaitCache[K, V]) Set(key K, val V) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.cache[key] == nil {
		ac.cache[key] = NewAwaitCacheValue[V]()
	}
	ac.cache[key].SetValue(val)
}

// Delete removes a key and its associated value from the cache.
func (ac *AwaitCache[K, V]) Delete(key K) {
	ac.mu.Lock()
	defer ac.mu.Unlock()

	if ac.cache[key] == nil {
		return
	}

	ac.cache[key].SetValue(*new(V))
	delete(ac.cache, key)
}

// AwaitCacheValue represents a value in the cache that can be awaited until it
// is ready.
type AwaitCacheValue[V any] struct {
	// The cached value
	value V

	// A channel used to signal when the value is ready
	readyChan chan struct{}

	// A synchronization primitive to ensure readiness is signaled only once
	readyOnce *sync.Once
}

// NewAwaitCacheValue creates and returns a new AwaitCacheValue instance.
func NewAwaitCacheValue[V any]() *AwaitCacheValue[V] {
	return &AwaitCacheValue[V]{
		readyChan: make(chan struct{}),
		readyOnce: new(sync.Once),
	}
}

// Await waits until the value is ready and returns it.
func (acv *AwaitCacheValue[V]) Await() V {
	<-acv.readyChan

	return acv.value
}

// SetValue sets the value of the AwaitCacheValue and signals its readiness.
func (acv *AwaitCacheValue[V]) SetValue(value V) {
	acv.value = value
	acv.readyOnce.Do(func() {
		close(acv.readyChan)
	})
}

// IsReady checks if the value is ready without blocking.
func (acv *AwaitCacheValue[V]) IsReady() bool {
	select {
	case <-acv.readyChan:
		return true
	default:
		return false
	}
}
