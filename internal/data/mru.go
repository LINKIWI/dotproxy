package data

import (
	"container/heap"
	"sync"
	"time"
)

// MRUQueue is an abstraction on top of a priority queue that assigns priorities based on
// timestamps, for most-recently-used retrieval semantics.
type MRUQueue struct {
	store    *PriorityQueue
	capacity int
	mutex    sync.Mutex
}

// NewMRUQueue creates a new MRU queue with the specified capacity.
// The capacity may be any non-positive integer to disable the capacity limit.
func NewMRUQueue(capacity int) *MRUQueue {
	var store PriorityQueue

	if capacity > 0 {
		store = make(PriorityQueue, 0, capacity)
	} else {
		store = make(PriorityQueue, 0)
	}

	heap.Init(&store)

	return &MRUQueue{store: &store, capacity: capacity}
}

// Push inserts a new value into the queue. It is tagged with a priority equal to the timestamp at
// which the item is inserted. It is considered an error to add an item beyond the queue's
// provisioned capacity.
func (m *MRUQueue) Push(value interface{}) bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Refuse to add beyond capacity
	if m.capacity > 0 && m.store.Len() == m.capacity {
		return false
	}

	heap.Push(m.store, &Item{
		value:    value,
		priority: int(time.Now().Unix()),
	})

	return true
}

// Pop removes the most recently used item from the queue. It returns the item itself, the timestamp
// at which it was last used, and a boolean indicating whether the pop was successful.
func (m *MRUQueue) Pop() (interface{}, time.Time, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.store.Len() == 0 {
		return nil, time.Unix(0, 0), false
	}

	item := heap.Pop(m.store).(*Item)
	return item.value, time.Unix(int64(item.priority), 0), true
}

// Size reads the current sizes of the queue.
func (m *MRUQueue) Size() int {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.store.Len()
}

// Empty returns whether the queue holds no items.
func (m *MRUQueue) Empty() bool {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	return m.store.Len() == 0
}
