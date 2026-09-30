package jira

import (
	"context"
	"fmt"
	"net/http"
)

// offsetPages GETs an offset-paginated endpoint (startAt/isLast/total/values)
// and returns every value. It stops on isLast, an empty page, or once
// startAt+len reaches total when the server sends one. More than maxPages
// pages is an error, so a server that ignores startAt cannot loop forever.
func offsetPages[T any](ctx context.Context, c *Client, path func(startAt int) string) ([]T, error) {
	var out []T
	start := 0
	for range maxPages {
		var r struct {
			IsLast bool `json:"isLast"`
			Total  *int `json:"total"`
			Values []T  `json:"values"`
		}
		if err := c.do(ctx, http.MethodGet, path(start), nil, &r); err != nil {
			return nil, err
		}
		out = append(out, r.Values...)
		start += len(r.Values)
		if r.IsLast || len(r.Values) == 0 || (r.Total != nil && start >= *r.Total) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("jira: %s: more than %d pages", path(0), maxPages)
}
