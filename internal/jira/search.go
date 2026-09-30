package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ericdahl-dev/jira-green/internal/model"
)

// maxPages caps every paginated call. Hitting it is an error, never a
// silently truncated result.
const maxPages = 50

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
	seen := map[string]bool{}
	for range maxPages {
		body := map[string]any{"jql": jql, "fields": fields, "maxResults": 100}
		if token != "" {
			body["nextPageToken"] = token
		}
		var r searchResp
		if err := c.do(ctx, http.MethodPost, "/rest/api/3/search/jql", body, &r); err != nil {
			return nil, err
		}
		for _, ai := range r.Issues {
			out = append(out, c.convert(ctx, ai, flaggedField))
		}
		if r.IsLast || r.NextPageToken == "" {
			return out, nil
		}
		if seen[r.NextPageToken] {
			return nil, fmt.Errorf("jira: search: nextPageToken %q repeated", r.NextPageToken)
		}
		seen[r.NextPageToken] = true
		token = r.NextPageToken
	}
	return nil, fmt.Errorf("jira: search: more than %d pages", maxPages)
}

type apiComment struct {
	Author  User `json:"author"`
	Created Time `json:"created"`
	Body    any  `json:"body"`
}

type commentPage struct {
	Total    int          `json:"total"`
	Comments []apiComment `json:"comments"`
}

type apiParent struct {
	Key    string `json:"key"`
	Fields struct {
		Summary string `json:"summary"`
	} `json:"fields"`
}

type apiStatus struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// fieldDecoder decodes issue fields one at a time, so one malformed field
// does not blank the others. It collects "name: err" for each failure.
type fieldDecoder struct {
	fields map[string]json.RawMessage
	errs   []string
}

// decodeField decodes fields[name] into a fresh T. Missing and null fields
// give the zero value and ok=false with no error; a decode failure gives the
// zero value (never a partial one), ok=false, and records the error.
func decodeField[T any](d *fieldDecoder, name string) (v T, ok bool) {
	raw, present := d.fields[name]
	if !present || string(raw) == "null" {
		return v, false
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		d.errs = append(d.errs, fmt.Errorf("%s: %w", name, err).Error())
		var zero T
		return zero, false
	}
	return v, true
}

func (c *Client) convert(ctx context.Context, ai apiIssue, flaggedField string) model.Issue {
	d := &fieldDecoder{fields: ai.Fields}
	iss := model.Issue{Key: ai.Key, URL: c.BrowseURL(ai.Key)}
	iss.Summary, _ = decodeField[string](d, "summary")
	if st, ok := decodeField[apiStatus](d, "status"); ok {
		iss.StatusID, iss.StatusName = st.ID, st.Name
	}
	if a, ok := decodeField[User](d, "assignee"); ok {
		iss.AssigneeID, iss.AssigneeName = a.AccountID, a.DisplayName
	}
	if p, ok := decodeField[apiParent](d, "parent"); ok {
		iss.EpicKey, iss.EpicSummary = p.Key, p.Fields.Summary
	}
	iss.Labels, _ = decodeField[[]string](d, "labels")
	if t, ok := decodeField[Time](d, "created"); ok {
		iss.Created = t.Time
	}
	if t, ok := decodeField[Time](d, "updated"); ok {
		iss.Updated = t.Time
	}
	if flaggedField != "" {
		flags, _ := decodeField[[]any](d, flaggedField)
		iss.Flagged = len(flags) > 0
	}
	cm, _ := decodeField[commentPage](d, "comment")
	if len(cm.Comments) < cm.Total {
		// Search embeds only the first page of comments; fetch the newest
		// 100 so a recent mention is not missed.
		var full commentPage
		path := "/rest/api/3/issue/" + url.PathEscape(ai.Key) + "/comment?orderBy=-created&maxResults=100"
		if err := c.do(ctx, http.MethodGet, path, nil, &full); err != nil {
			d.errs = append(d.errs, fmt.Sprintf("comments: truncated, fetch failed: %v", err))
		} else {
			cm = full
		}
	}
	for _, cm := range cm.Comments {
		iss.Comments = append(iss.Comments, model.Comment{
			AuthorID: cm.Author.AccountID,
			Created:  cm.Created.Time,
			Mentions: mentions(cm.Body),
		})
	}
	iss.DecodeErrors = d.errs
	return iss
}
