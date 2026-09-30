package jira_test

import (
	"context"
	"encoding/json"
	"net/http"
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
	if err != nil || len(cols) != 2 || cols[1].Name != "Code Review" || cols[1].StatusIDs[1] != "11" {
		t.Fatalf("%+v %v", cols, err)
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
