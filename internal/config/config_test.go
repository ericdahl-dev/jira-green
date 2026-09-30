package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const minimal = `
[jira]
  site = "https://example.atlassian.net"
  email = "me@example.com"
  token_env = "JG_TEST_TOKEN"
  board_id = 7
`

func TestLoadDefaults(t *testing.T) {
	c, err := Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if c.Settings.PollIntervalSeconds != 60 || c.Settings.BoardRefreshIntervalSeconds != 600 {
		t.Errorf("settings %+v", c.Settings)
	}
	if c.Settings.DefaultView != "kanban" {
		t.Errorf("view %q", c.Settings.DefaultView)
	}
	if c.Settings.StuckAlertAfter != "2h" {
		t.Errorf("stuck_alert_after %q", c.Settings.StuckAlertAfter)
	}
	if c.JQL.Mine == "" || c.JQL.Waiting == "" || c.JQL.Done == "" {
		t.Errorf("default JQL missing: %+v", c.JQL)
	}
	r, err := c.Rules("acct-me")
	if err != nil {
		t.Fatal(err)
	}
	if r.Thresholds["In Progress"].Red != 5*24*time.Hour || r.Thresholds["Code Review"].Yellow != 24*time.Hour {
		t.Errorf("default thresholds %+v", r.Thresholds)
	}
	if r.Me != "acct-me" || len(r.BlockedLabels) != 1 || r.BlockedLabels[0] != "blocked" {
		t.Errorf("rules %+v", r)
	}
}

func TestLoadRejects(t *testing.T) {
	for name, body := range map[string]string{
		"no site":   `[jira]` + "\n" + `board_id = 1`,
		"no board":  `[jira]` + "\n" + `site = "https://example.atlassian.net"`,
		"bad view":  minimal + "\n[settings]\n  default_view = \"grid\"\n",
		"bad stuck": minimal + "\n[settings]\n  stuck_alert_after = \"later\"\n",
		"bad age":   minimal + "\n[thresholds.\"UA\"]\n  yellow = \"soon\"\n",
	} {
		if _, err := Load(write(t, body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestResolveTokenOrder(t *testing.T) {
	t.Setenv("JG_TEST_TOKEN", "from-env")
	t.Setenv("JIRA_API_TOKEN", "fallback")
	if tok, _ := ResolveToken(Jira{TokenCommand: "printf from-cmd", TokenEnv: "JG_TEST_TOKEN"}); tok != "from-cmd" {
		t.Errorf("cmd first, got %q", tok)
	}
	if tok, _ := ResolveToken(Jira{TokenEnv: "JG_TEST_TOKEN"}); tok != "from-env" {
		t.Errorf("env second, got %q", tok)
	}
	if tok, _ := ResolveToken(Jira{Token: "literal"}); tok != "literal" {
		t.Errorf("literal third, got %q", tok)
	}
	if tok, _ := ResolveToken(Jira{}); tok != "fallback" {
		t.Errorf("JIRA_API_TOKEN last, got %q", tok)
	}
	t.Setenv("JIRA_API_TOKEN", "")
	if _, err := ResolveToken(Jira{}); err == nil {
		t.Error("want error with no token source")
	}
}

func TestResolveTokenCommandErrors(t *testing.T) {
	for name, cmd := range map[string]string{
		"fails":     "echo nope >&2; exit 3",
		"no output": "true",
	} {
		if _, err := ResolveToken(Jira{TokenCommand: cmd, Token: "literal"}); err == nil {
			t.Errorf("%s: want error, not a fall-through", name)
		}
	}
}

func TestMuteRoundTrip(t *testing.T) {
	c, _ := Load(write(t, minimal))
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(c.Path())
	if !c2.IsMuted("ABC-9") || c2.IsMuted("ABC-1") {
		t.Errorf("mutes %v", c2.Muted)
	}
}

func TestUnmuteRoundTrip(t *testing.T) {
	c, _ := Load(write(t, "muted = [\"ABC-9\", \"ABC-2\"]\n"+minimal))
	if err := c.SetMuted("ABC-9", false); err != nil {
		t.Fatal(err)
	}
	c2, _ := Load(c.Path())
	if c2.IsMuted("ABC-9") || !c2.IsMuted("ABC-2") {
		t.Errorf("mutes %v", c2.Muted)
	}
}

func TestLoadTrimsSiteSlash(t *testing.T) {
	c, err := Load(write(t, "[jira]\n  site = \"https://example.atlassian.net/\"\n  board_id = 7\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Jira.Site != "https://example.atlassian.net" {
		t.Errorf("site %q", c.Jira.Site)
	}
}

func TestSaveKeepsWebhooks(t *testing.T) {
	body := minimal + "\n[[webhooks]]\n  url = \"https://hooks.example.com/x\"\n  secret = \"s\"\n"
	c, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(c2.Webhooks) != 1 || c2.Webhooks[0].URL != "https://hooks.example.com/x" || c2.Webhooks[0].Secret != "s" {
		t.Errorf("webhooks %+v", c2.Webhooks)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p := DefaultPath(); p != filepath.Join("/xdg", "jira-green", "config.toml") {
		t.Errorf("xdg path %q", p)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/u")
	if p := DefaultPath(); p != filepath.Join("/home/u", ".config", "jira-green", "config.toml") {
		t.Errorf("home path %q", p)
	}
}

func TestWriteStarter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	j := Jira{Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JG_TEST_TOKEN", BoardID: 7}
	if _, err := WriteStarter(p, j); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Jira != j || c.Settings.DefaultView != "kanban" || len(c.Thresholds) == 0 {
		t.Errorf("starter %+v", c)
	}
	if _, err := WriteStarter(p, j); err == nil {
		t.Error("want error: starter must not overwrite")
	}
}
