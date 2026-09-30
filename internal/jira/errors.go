package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// APIError is a non-2xx response.
type APIError struct {
	Status int
	// Messages is Jira's error envelope flattened: errorMessages first, then
	// "field: msg" for each entry of errors, sorted by field.
	Messages []string
	// Body is the raw (size-limited) response body, kept for debugging.
	Body       string
	RetryAfter time.Duration
}

// maxErrorLen caps Error() so a verbose Jira response fits a status line.
const maxErrorLen = 200

// Error is "jira: HTTP 400: msg1; field: msg2", cut to about 200
// characters, or "jira: HTTP 502 Bad Gateway" when the body had no Jira
// error envelope (an HTML proxy page, say).
func (e *APIError) Error() string {
	if len(e.Messages) == 0 {
		return strings.TrimSpace(fmt.Sprintf("jira: HTTP %d %s", e.Status, http.StatusText(e.Status)))
	}
	s := fmt.Sprintf("jira: HTTP %d: %s", e.Status, strings.Join(e.Messages, "; "))
	if r := []rune(s); len(r) > maxErrorLen {
		s = string(r[:maxErrorLen-1]) + "…"
	}
	return s
}

// errorMessages parses Jira's {"errorMessages":[...],"errors":{...}}
// envelope. It returns nil when body is not such an envelope.
func errorMessages(body []byte) []string {
	var env struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if json.Unmarshal(body, &env) != nil {
		return nil
	}
	out := slices.Clone(env.ErrorMessages)
	for _, k := range slices.Sorted(maps.Keys(env.Errors)) {
		out = append(out, k+": "+env.Errors[k])
	}
	return out
}

// IsAuth reports whether err is a 401 Unauthorized: the credentials are bad,
// so polling should stop rather than retry. A 403 Forbidden is deliberately
// not an auth error: it usually means one resource (a board, an issue) is
// off-limits while the token still works, so polling keeps going.
func IsAuth(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusUnauthorized
}

// defaultRetryAfter is the backoff for a 429 with no usable Retry-After.
const defaultRetryAfter = 60 * time.Second

// parseRetryAfter reads a Retry-After header: whole seconds or an HTTP-date.
// It returns 0 for a missing, unparseable, or negative value.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	var d time.Duration
	if n, err := strconv.Atoi(h); err == nil {
		d = time.Duration(n) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		d = time.Until(t)
	}
	return max(d, 0)
}
