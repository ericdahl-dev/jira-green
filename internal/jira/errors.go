package jira

import (
	"errors"
	"fmt"
	"net/http"
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

// IsAuth reports whether err is a 401 Unauthorized: the credentials are bad,
// so polling should stop rather than retry. A 403 Forbidden is deliberately
// not an auth error: it usually means one resource (a board, an issue) is
// off-limits while the token still works, so polling keeps going.
func IsAuth(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

func parseRetryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(h); err == nil {
		return time.Duration(n) * time.Second
	}
	return 0
}
