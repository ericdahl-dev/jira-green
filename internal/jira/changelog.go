package jira

import (
	"context"
	"fmt"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

type apiHistory struct {
	Created Time `json:"created"`
	Items   []struct {
		Field string  `json:"field"`
		To    *string `json:"to"`
	} `json:"items"`
}

// StatusChanges returns every status transition in the issue's changelog.
func (c *Client) StatusChanges(ctx context.Context, key string) ([]model.StatusChange, error) {
	hs, err := offsetPages[apiHistory](ctx, c, func(start int) string {
		return fmt.Sprintf("/rest/api/3/issue/%s/changelog?startAt=%d&maxResults=100", key, start)
	})
	if err != nil {
		return nil, err
	}
	var out []model.StatusChange
	for _, h := range hs {
		for _, it := range h.Items {
			if it.Field == "status" && it.To != nil {
				out = append(out, model.StatusChange{At: h.Created.Time, ToID: *it.To})
			}
		}
	}
	return out, nil
}
