package alert_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/alert"
	"github.com/ericdahl-dev/jira-green/internal/config"
)

func TestDispatchPostsSignedJSON(t *testing.T) {
	var (
		mu       sync.Mutex
		gotBody  []byte
		gotSig   string
		gotType  string
		received = make(chan struct{}, 1)
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		gotBody, gotSig, gotType = body, r.Header.Get("X-Coolify-Green-Signature"), r.Header.Get("Content-Type")
		mu.Unlock()
		received <- struct{}{}
	}))
	defer srv.Close()

	d := alert.New([]config.Webhook{{URL: srv.URL, Secret: "shh"}})
	err := d.Dispatch(context.Background(), alert.Event{
		Type:    alert.TypeTicketStuck,
		Key:     "ABC-1",
		Summary: "Fix the widget",
		Status:  "In Progress",
		URL:     "https://example.atlassian.net/browse/ABC-1",
		Reasons: []string{"flagged"},
		RedFor:  "2h",
		At:      time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Errorf("dispatch: %v", err)
	}

	select {
	case <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not delivered")
	}

	mu.Lock()
	defer mu.Unlock()
	if gotType != "application/json" {
		t.Errorf("content-type = %q", gotType)
	}
	var decoded map[string]any
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if decoded["type"] != "ticket.stuck" || decoded["key"] != "ABC-1" || decoded["red_for"] != "2h" {
		t.Errorf("decoded = %+v", decoded)
	}
	mac := hmac.New(sha256.New, []byte("shh"))
	mac.Write(gotBody)
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); gotSig != want {
		t.Errorf("signature = %q, want %q", gotSig, want)
	}
}

func TestDispatchWithoutSecretSendsNoSignature(t *testing.T) {
	done := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		done <- r.Header.Get(alert.SignatureHeader)
	}))
	defer srv.Close()

	if err := alert.New([]config.Webhook{{URL: srv.URL}}).Dispatch(context.Background(), alert.Event{Type: alert.TypeTicketStuck}); err != nil {
		t.Errorf("dispatch: %v", err)
	}
	select {
	case sig := <-done:
		if sig != "" {
			t.Errorf("unsigned request carried a signature: %q", sig)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not delivered")
	}
}

func TestDispatchReportsFailureWithoutSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	hooks := []config.Webhook{{URL: srv.URL + "/hook?token=hunter2", Secret: "shh"}}
	err := alert.New(hooks).Dispatch(context.Background(), alert.Event{Type: alert.TypeTicketStuck})
	if err == nil {
		t.Fatal("a 500 was not reported")
	}
	msg := err.Error()
	if msg != u.Host+": 500" {
		t.Errorf("error %q, want %q", msg, u.Host+": 500")
	}
	for _, leak := range []string{"hunter2", "token", "shh", "/hook"} {
		if strings.Contains(msg, leak) {
			t.Errorf("error %q leaks %q", msg, leak)
		}
	}
}

func TestDispatchNoHooksIsNoop(t *testing.T) {
	if err := alert.New(nil).Dispatch(context.Background(), alert.Event{Type: alert.TypeTicketStuck}); err != nil {
		t.Errorf("no hooks: %v", err)
	}
}

func TestDispatchCanceledContextSendsNothing(t *testing.T) {
	hit := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := alert.New([]config.Webhook{{URL: srv.URL}}).Dispatch(ctx, alert.Event{Type: alert.TypeTicketStuck}); err == nil {
		t.Error("a canceled dispatch reported success")
	}
	select {
	case <-hit:
		t.Fatal("a canceled context still delivered the webhook")
	case <-time.After(100 * time.Millisecond):
	}
}
