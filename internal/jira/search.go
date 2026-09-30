package jira

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

var baseFields = []string{"summary", "status", "assignee", "parent", "labels", "created", "updated", "comment"}

type apiIssue struct {
	Key    string                     `json:"key"`
	Fields map[string]json.RawMessage `json:"fields"`
}

type searchResp struct {
	Issues        []apiIssue `json:"issues"`
	NextPageToken string     `json:"nextPageToken"`
	IsLast        bool       `json:"isLast"`
}

// Search runs JQL and returns every matching issue as a model.Issue.
// flaggedField is the custom field ID for "Flagged" ("" to skip).
func (c *Client) Search(ctx context.Context, jql, flaggedField string) ([]model.Issue, error) {
	fields := append([]string{}, baseFields...)
	if flaggedField != "" {
		fields = append(fields, flaggedField)
	}
	var out []model.Issue
	token := ""
	for {
		body := map[string]any{"jql": jql, "fields": fields, "maxResults": 100}
		if token != "" {
			body["nextPageToken"] = token
		}
		var r searchResp
		if err := c.do(ctx, http.MethodPost, "/rest/api/3/search/jql", body, &r); err != nil {
			return nil, err
		}
		for _, ai := range r.Issues {
			out = append(out, c.convert(ai, flaggedField))
		}
		if r.IsLast || r.NextPageToken == "" {
			return out, nil
		}
		token = r.NextPageToken
	}
}

func (c *Client) convert(ai apiIssue, flaggedField string) model.Issue {
	var f struct {
		Summary string `json:"summary"`
		Status  struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"status"`
		Assignee *User `json:"assignee"`
		Parent   *struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
			} `json:"fields"`
		} `json:"parent"`
		Labels  []string `json:"labels"`
		Created Time     `json:"created"`
		Updated Time     `json:"updated"`
		Comment struct {
			Comments []struct {
				Author  User `json:"author"`
				Created Time `json:"created"`
				Body    any  `json:"body"`
			} `json:"comments"`
		} `json:"comment"`
	}
	raw, _ := json.Marshal(ai.Fields)
	_ = json.Unmarshal(raw, &f)

	iss := model.Issue{
		Key:        ai.Key,
		Summary:    f.Summary,
		URL:        c.BrowseURL(ai.Key),
		StatusID:   f.Status.ID,
		StatusName: f.Status.Name,
		Labels:     f.Labels,
		Created:    f.Created.Time,
		Updated:    f.Updated.Time,
	}
	if f.Assignee != nil {
		iss.AssigneeID, iss.AssigneeName = f.Assignee.AccountID, f.Assignee.DisplayName
	}
	if f.Parent != nil {
		iss.EpicKey, iss.EpicSummary = f.Parent.Key, f.Parent.Fields.Summary
	}
	if flaggedField != "" {
		var flags []any
		if json.Unmarshal(ai.Fields[flaggedField], &flags) == nil && len(flags) > 0 {
			iss.Flagged = true
		}
	}
	for _, cm := range f.Comment.Comments {
		iss.Comments = append(iss.Comments, model.Comment{
			AuthorID: cm.Author.AccountID,
			Created:  cm.Created.Time,
			Mentions: mentions(cm.Body),
		})
	}
	return iss
}
