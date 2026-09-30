package alert_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
	d.Dispatch(alert.Event{
		Type:    alert.TypeTicketStuck,
		Key:     "ABC-1",
		Summary: "Fix the widget",
		Status:  "In Progress",
		URL:     "https://example.atlassian.net/browse/ABC-1",
		Reasons: []string{"flagged"},
		RedFor:  "2h",
		At:      time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC),
	})

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

	alert.New([]config.Webhook{{URL: srv.URL}}).Dispatch(alert.Event{Type: alert.TypeTicketStuck})
	select {
	case sig := <-done:
		if sig != "" {
			t.Errorf("unsigned request carried a signature: %q", sig)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not delivered")
	}
}

func TestDispatchSurvivesFailingEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	// Must not panic and must not block.
	alert.New([]config.Webhook{{URL: srv.URL}}).Dispatch(alert.Event{Type: alert.TypeTicketStuck})
}

func TestDispatchNoHooksIsNoop(t *testing.T) {
	alert.New(nil).Dispatch(alert.Event{Type: alert.TypeTicketStuck})
}
