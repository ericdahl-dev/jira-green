package jira

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// APIError is a non-2xx response.
type APIError struct {
	Status     int
	Body       string
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jira: HTTP %d: %s", e.Status, e.Body)
}

// IsAuth reports whether err is a 401/403: polling should stop, not retry.
func IsAuth(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == 401 || ae.Status == 403)
}

func parseRetryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(h); err == nil {
		return time.Duration(n) * time.Second
	}
	return 0
}
