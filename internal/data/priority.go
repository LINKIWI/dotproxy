package data

import (
	"container/heap"
)

// Item describes an entry in the priority queue.
type Item struct {
	value    interface{}
	priority int
	index    int
}

// PriorityQueue implements heap.Interface and holds Items.
// This implementation is adapted from the container/heap documentation:
// https://golang.org/pkg/container/heap/
type PriorityQueue []*Item

// Len returns the current size of the queue.
func (pq PriorityQueue) Len() int {
	return len(pq)
}

// Less instructs heap.Interface how to sort items within the heap.
// A priority queue is a max heap, so this particular application considers a higher priority as
// "less." This allows us to pop the highest-priority item.
func (pq PriorityQueue) Less(i, j int) bool {
	return pq[i].priority > pq[j].priority
}

// Swap swaps the ith and jth items in the backing data structure.
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

// Push adds a new item to the backing data structure.
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

// Pop removes the last item from the backing data structure.
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1
	*pq = old[0 : n-1]

	return item
}

// update modifies the priority and value of an Item in the queue.
func (pq *PriorityQueue) update(item *Item, value string, priority int) {
	item.value = value
	item.priority = priority
	heap.Fix(pq, item.index)
}
