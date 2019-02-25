package metrics

import (
	"time"
)

// Timer is a simple abstraction to help measure execution durations.
type Timer struct {
	start time.Time
}

// NewTimer creates and starts an execution timer.
func NewTimer() *Timer {
	return &Timer{
		start: time.Now(),
	}
}

// Elapsed returns the amount of time that has elapsed since the timer has started.
func (t *Timer) Elapsed() time.Duration {
	return time.Since(t.start)
}
