package jira_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"
)

func TestBoardColumns(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board/7/configuration" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"columnConfig":{"columns":[
			{"name":"To Do","statuses":[{"id":"1"}]},
			{"name":"Code Review","statuses":[{"id":"10"},{"id":"11"}]}]}}`))
	})
	cols, err := c.BoardColumns(context.Background(), 7)
	if err != nil || len(cols) != 2 {
		t.Fatalf("%+v %v", cols, err)
	}
	if cols[0].Name != "To Do" || !slices.Equal(cols[0].StatusIDs, []string{"1"}) ||
		cols[1].Name != "Code Review" || !slices.Equal(cols[1].StatusIDs, []string{"10", "11"}) {
		t.Errorf("%+v", cols)
	}
}

func TestStatusChangesPaginates(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/issue/ABC-1/changelog" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("startAt") == "0" {
			_, _ = w.Write([]byte(`{"startAt":0,"maxResults":1,"isLast":false,"values":[
				{"created":"2026-09-21T09:00:00.000-0400","items":[{"field":"status","to":"3"},{"field":"labels","to":null}]}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"startAt":1,"maxResults":1,"isLast":true,"values":[
			{"created":"2026-09-22T09:00:00.000-0400","items":[{"field":"status","to":"10"}]}]}`))
	})
	ch, err := c.StatusChanges(context.Background(), "ABC-1")
	if err != nil || len(ch) != 2 || ch[0].ToID != "3" || ch[1].ToID != "10" {
		t.Fatalf("%+v %v", ch, err)
	}
	if !ch[1].At.Equal(time.Date(2026, 9, 22, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("at %v", ch[1].At)
	}
}

func TestTransitions(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/rest/api/3/issue/ABC-1/transitions" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"transitions":[{"id":"21","name":"Start review","to":{"id":"10","name":"Code Review"}}]}`))
	})
	ts, err := c.Transitions(context.Background(), "ABC-1")
	if err != nil || len(ts) != 1 || ts[0].ID != "21" || ts[0].Name != "Start review" || ts[0].ToName != "Code Review" {
		t.Fatalf("%+v %v", ts, err)
	}
}

func TestDoTransitionPostsID(t *testing.T) {
	var posted map[string]any
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/3/issue/ABC-1/transitions" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	if err := c.DoTransition(context.Background(), "ABC-1", "21"); err != nil {
		t.Fatal(err)
	}
	if tr, _ := posted["transition"].(map[string]any); tr["id"] != "21" {
		t.Errorf("posted %v", posted)
	}
}

func TestFindFieldID(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/3/field" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"id":"summary","name":"Summary"},{"id":"customfield_10021","name":"Flagged"}]`))
	})
	id, err := c.FindFieldID(context.Background(), "flagged")
	if err != nil || id != "customfield_10021" {
		t.Fatalf("%q %v", id, err)
	}
	id, err = c.FindFieldID(context.Background(), "Story Points")
	if err != nil || id != "" {
		t.Fatalf("missing field: %q %v", id, err)
	}
}

func TestBoardsPaginates(t *testing.T) {
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/agile/1.0/board" {
			t.Errorf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("startAt") == "0" {
			_, _ = w.Write([]byte(`{"isLast":false,"values":[{"id":7,"name":"ABC board","location":{"projectKey":"ABC"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":8,"name":"ABC ops","location":{"projectKey":"ABC"}}]}`))
	})
	bs, err := c.Boards(context.Background())
	if err != nil || len(bs) != 2 || bs[0].ID != 7 || bs[0].Name != "ABC board" || bs[0].ProjectKey != "ABC" || bs[1].ID != 8 {
		t.Fatalf("%+v %v", bs, err)
	}
}

func TestBoardsServerIgnoringStartAtHitsPageCap(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 60 {
			t.Errorf("still paginating after %d calls", calls)
			_, _ = w.Write([]byte(`{"isLast":true,"values":[]}`))
			return
		}
		_, _ = w.Write([]byte(`{"isLast":false,"values":[{"id":7,"name":"ABC board"}]}`))
	})
	if _, err := c.Boards(context.Background()); err == nil {
		t.Fatal("want an error once the page cap is exceeded")
	}
	if calls != 50 {
		t.Errorf("calls %d, want the 50-page cap", calls)
	}
}

func TestStatusChangesStopsAtTotal(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls > 60 {
			_, _ = w.Write([]byte(`{"isLast":true,"values":[]}`))
			return
		}
		// No isLast, and startAt ignored: only total says we are done.
		_, _ = w.Write([]byte(`{"startAt":0,"total":1,"values":[
			{"created":"2026-09-21T09:00:00.000-0400","items":[{"field":"status","to":"3"}]}]}`))
	})
	ch, err := c.StatusChanges(context.Background(), "ABC-1")
	if err != nil || len(ch) != 1 || calls != 1 {
		t.Fatalf("changes %+v err %v calls %d", ch, err, calls)
	}
}

func TestBoardsEmptyPageEndsPagination(t *testing.T) {
	calls := 0
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Query().Get("startAt") == "0" {
			_, _ = w.Write([]byte(`{"isLast":false,"values":[{"id":7,"name":"ABC board"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"isLast":false,"values":[]}`))
	})
	bs, err := c.Boards(context.Background())
	if err != nil || len(bs) != 1 || calls != 2 {
		t.Fatalf("boards %+v err %v calls %d", bs, err, calls)
	}
}

func TestIssueKeysArePathEscaped(t *testing.T) {
	var paths []string
	c := newTest(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		switch {
		case r.URL.Path == "/rest/api/3/search/jql":
			_, _ = w.Write([]byte(`{"isLast":true,"issues":[{"key":"A/B-1","fields":{"comment":{"total":2,"comments":[]}}}]}`))
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`{"isLast":true}`))
		}
	})
	ctx := context.Background()
	_, _ = c.StatusChanges(ctx, "A/B-1")
	_, _ = c.Transitions(ctx, "A/B-1")
	_ = c.DoTransition(ctx, "A/B-1", "21")
	_, _ = c.Search(ctx, "project = ABC", "")
	_, _ = c.Comments(ctx, "A/B-1")
	want := []string{
		"/rest/api/3/issue/A%2FB-1/changelog",
		"/rest/api/3/issue/A%2FB-1/transitions",
		"/rest/api/3/issue/A%2FB-1/transitions",
		"/rest/api/3/search/jql",
		"/rest/api/3/issue/A%2FB-1/comment",
	}
	if !slices.Equal(paths, want) {
		t.Errorf("paths\n got %q\nwant %q", paths, want)
	}
}
