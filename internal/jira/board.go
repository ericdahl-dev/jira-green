package jira

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// BoardColumns returns the board's columns in display order.
func (c *Client) BoardColumns(ctx context.Context, boardID int) ([]model.Column, error) {
	var r struct {
		ColumnConfig struct {
			Columns []struct {
				Name     string `json:"name"`
				Statuses []struct {
					ID string `json:"id"`
				} `json:"statuses"`
			} `json:"columns"`
		} `json:"columnConfig"`
	}
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/rest/agile/1.0/board/%d/configuration", boardID), nil, &r); err != nil {
		return nil, err
	}
	var cols []model.Column
	for _, col := range r.ColumnConfig.Columns {
		mc := model.Column{Name: col.Name}
		for _, s := range col.Statuses {
			mc.StatusIDs = append(mc.StatusIDs, s.ID)
		}
		cols = append(cols, mc)
	}
	return cols, nil
}

// Board is an agile board the user can see.
type Board struct {
	ID         int
	Name       string
	ProjectKey string
}

type apiBoard struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Location struct {
		ProjectKey string `json:"projectKey"`
	} `json:"location"`
}

// Boards lists boards visible to the user (for the init wizard).
func (c *Client) Boards(ctx context.Context) ([]Board, error) {
	vs, err := offsetPages[apiBoard](ctx, c, func(start int) string {
		return fmt.Sprintf("/rest/agile/1.0/board?startAt=%d&maxResults=50", start)
	})
	if err != nil {
		return nil, err
	}
	out := make([]Board, 0, len(vs))
	for _, v := range vs {
		out = append(out, Board{ID: v.ID, Name: v.Name, ProjectKey: v.Location.ProjectKey})
	}
	return out, nil
}
