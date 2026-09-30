package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// StatusChanges returns every status transition in the issue's changelog.
func (c *Client) StatusChanges(ctx context.Context, key string) ([]model.StatusChange, error) {
	var out []model.StatusChange
	for start := 0; ; {
		var r struct {
			IsLast bool `json:"isLast"`
			Values []struct {
				Created Time `json:"created"`
				Items   []struct {
					Field string  `json:"field"`
					To    *string `json:"to"`
				} `json:"items"`
			} `json:"values"`
		}
		path := fmt.Sprintf("/rest/api/3/issue/%s/changelog?startAt=%d&maxResults=100", key, start)
		if err := c.do(ctx, http.MethodGet, path, nil, &r); err != nil {
			return nil, err
		}
		for _, v := range r.Values {
			for _, it := range v.Items {
				if it.Field == "status" && it.To != nil {
					out = append(out, model.StatusChange{At: v.Created.Time, ToID: *it.To})
				}
			}
		}
		if r.IsLast || len(r.Values) == 0 {
			return out, nil
		}
		start += len(r.Values)
	}
}
