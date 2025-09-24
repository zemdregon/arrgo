package timeutil

import "time"

// Clock abstracts time retrieval for deterministic testing.
type Clock interface {
	Now() time.Time
}

// SystemClock returns the current wall clock time in UTC.
type SystemClock struct{}

// Now returns the current time in UTC.
func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
