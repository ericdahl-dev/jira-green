package jira

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// BoardConfig is the part of a board's configuration jira-green reads.
type BoardConfig struct {
	Columns []model.Column // display order
	// FilterID is the board's saved filter, for scoping lane JQL with
	// "filter = ID". It is "" when the response had none, or an ID that is
	// not all digits (it is spliced into JQL, so it must be a bare number).
	FilterID string
}

// BoardConfig returns the board's columns and saved filter ID.
func (c *Client) BoardConfig(ctx context.Context, boardID int) (BoardConfig, error) {
	var r struct {
		Filter struct {
			ID string `json:"id"`
		} `json:"filter"`
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
		return BoardConfig{}, err
	}
	bc := BoardConfig{}
	if isDigits(r.Filter.ID) {
		bc.FilterID = r.Filter.ID
	}
	for _, col := range r.ColumnConfig.Columns {
		mc := model.Column{Name: col.Name}
		for _, s := range col.Statuses {
			mc.StatusIDs = append(mc.StatusIDs, s.ID)
		}
		bc.Columns = append(bc.Columns, mc)
	}
	return bc, nil
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

func isDigits(s string) bool {
	return s != "" && strings.Trim(s, "0123456789") == ""
}
