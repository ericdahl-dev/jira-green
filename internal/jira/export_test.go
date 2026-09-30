package jira

import "time"

// SetNow replaces the client's clock for a test.
func SetNow(c *Client, now func() time.Time) { c.now = now }
