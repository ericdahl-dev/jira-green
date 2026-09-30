package ui

import "time"

// SetFlashFor sets how long a flash stays, returning the old value.
func SetFlashFor(d time.Duration) time.Duration { old := flashFor; flashFor = d; return old }
