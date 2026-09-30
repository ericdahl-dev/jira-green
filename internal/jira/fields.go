package jira

import (
	"context"
	"net/http"
	"strings"
)

// FindFieldID returns the ID of the field with the given display name
// (case-insensitive), or "" if none.
func (c *Client) FindFieldID(ctx context.Context, name string) (string, error) {
	var fs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := c.do(ctx, http.MethodGet, "/rest/api/3/field", nil, &fs); err != nil {
		return "", err
	}
	for _, f := range fs {
		if strings.EqualFold(f.Name, name) {
			return f.ID, nil
		}
	}
	return "", nil
}
