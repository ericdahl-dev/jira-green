package poller

import "time"

// SetNow replaces the poller's clock for a test.
func SetNow(p *Poller, now func() time.Time) { p.now = now }

// SetAfter replaces the timer Start waits on between polls.
func SetAfter(p *Poller, after func(time.Duration) <-chan time.Time) { p.after = after }
