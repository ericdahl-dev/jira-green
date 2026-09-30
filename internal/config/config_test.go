package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	if c.PollInterval() != 60*time.Second || c.BoardRefreshInterval() != 10*time.Minute {
		t.Errorf("intervals %v %v", c.PollInterval(), c.BoardRefreshInterval())
	}
	if c.DefaultView() != "kanban" {
		t.Errorf("view %q", c.DefaultView())
	}
	if c.StuckAlertAfter() != 2*time.Hour {
		t.Errorf("stuck_alert_after %v", c.StuckAlertAfter())
	}
	if c.MineJQL() != DefaultMineJQL || c.WaitingJQL() != DefaultWaitingJQL || c.DoneJQL() != DefaultDoneJQL {
		t.Errorf("default JQL %q %q %q", c.MineJQL(), c.WaitingJQL(), c.DoneJQL())
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
	if c.Jira != j || c.DefaultView() != "kanban" {
		t.Errorf("starter %+v", c)
	}
	if r, err := c.Rules(""); err != nil || r.Thresholds["In Progress"].Red != 5*24*time.Hour {
		t.Errorf("starter rules %+v, %v", r, err)
	}
	if _, err := WriteStarter(p, j); err == nil {
		t.Error("want error: starter must not overwrite")
	}
}

func TestThresholdsMergeKeepsOtherDefaults(t *testing.T) {
	c, err := Load(write(t, minimal+"\n[thresholds.\"In Progress\"]\n  yellow = \"4d\"\n  red = \"6d\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if r.Thresholds["In Progress"].Yellow != 4*24*time.Hour || r.Thresholds["In Progress"].Red != 6*24*time.Hour {
		t.Errorf("in progress %+v", r.Thresholds["In Progress"])
	}
	if r.Thresholds["UA"].Red != 2*24*time.Hour || r.Thresholds["Code Review"].Yellow != 24*time.Hour {
		t.Errorf("defaults lost: %+v", r.Thresholds)
	}
}

func TestThresholdsOverrideReplacesColumn(t *testing.T) {
	c, err := Load(write(t, minimal+"\n[thresholds.\"Code Review\"]\n  yellow = \"4h\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if th := r.Thresholds["Code Review"]; th.Yellow != 4*time.Hour || th.Red != 0 {
		t.Errorf("code review %+v, want yellow 4h and no red", th)
	}
}

func TestThresholdsEmptyEntryDisablesColumn(t *testing.T) {
	c, err := Load(write(t, minimal+"\n[thresholds.\"UA\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if th, ok := r.Thresholds["UA"]; !ok || th.Yellow != 0 || th.Red != 0 {
		t.Errorf("UA %+v (present %v), want zero threshold", th, ok)
	}
	if r.Thresholds["In Progress"].Red != 5*24*time.Hour {
		t.Errorf("in progress default lost: %+v", r.Thresholds)
	}
}

func TestThresholdsAddsUserColumn(t *testing.T) {
	c, err := Load(write(t, minimal+"\n[thresholds.\"QA\"]\n  red = \"3d\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if r.Thresholds["QA"].Red != 3*24*time.Hour {
		t.Errorf("QA %+v", r.Thresholds["QA"])
	}
	if len(r.Thresholds) != 4 {
		t.Errorf("want 3 defaults + QA, got %+v", r.Thresholds)
	}
}

func TestThresholdsSaveRoundTrip(t *testing.T) {
	body := minimal + "\n[thresholds.\"UA\"]\n\n[thresholds.\"QA\"]\n  red = \"3d\"\n"
	c, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	want, _ := c.Rules("")
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, _ := c2.Rules("")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after save got %+v, want %+v", got.Thresholds, want.Thresholds)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	_, err := Load(write(t, minimal+"  token_cmd = \"pass jira\"\n"))
	if err == nil || !strings.Contains(err.Error(), "jira.token_cmd") {
		t.Errorf("want unknown-key error naming jira.token_cmd, got %v", err)
	}
}

func TestLoadRejectsMisplacedMuted(t *testing.T) {
	_, err := Load(write(t, minimal+"muted = [\"ABC-9\"]\n"))
	if err == nil || !strings.Contains(err.Error(), "jira.muted") {
		t.Errorf("want unknown-key error naming jira.muted, got %v", err)
	}
}

func TestSaveKeepsOnlyWhatUserWrote(t *testing.T) {
	c, err := Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"[thresholds", "[jql]", "blocked_labels", "[settings]"} {
		if strings.Contains(string(b), absent) {
			t.Errorf("saved file contains %q, want only user-written keys:\n%s", absent, b)
		}
	}
}

func TestEmptyBlockedLabelsSurvivesSave(t *testing.T) {
	c, err := Load(write(t, "blocked_labels = []\n"+minimal))
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
	r, err := c2.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.BlockedLabels) != 0 {
		t.Errorf("blocked labels %v, want none (user disabled them)", r.BlockedLabels)
	}
}

func TestAccessorsReturnUserValues(t *testing.T) {
	body := `
[settings]
  poll_interval_seconds = 30
  board_refresh_interval_seconds = 120
  stuck_alert_after = "1d"
  default_view = "list"
[jira]
  site = "https://example.atlassian.net"
  email = "me@example.com"
  board_id = 7
[jql]
  mine = "project = ABC"
  waiting = "project = ABC AND reporter = currentUser()"
  done = "project = ABC AND statusCategory = Done"
`
	c, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if c.PollInterval() != 30*time.Second || c.BoardRefreshInterval() != 2*time.Minute || c.StuckAlertAfter() != 24*time.Hour || c.DefaultView() != "list" {
		t.Errorf("settings %v %v %v %q", c.PollInterval(), c.BoardRefreshInterval(), c.StuckAlertAfter(), c.DefaultView())
	}
	if c.MineJQL() != "project = ABC" || c.WaitingJQL() != "project = ABC AND reporter = currentUser()" || c.DoneJQL() != "project = ABC AND statusCategory = Done" {
		t.Errorf("jql %q %q %q", c.MineJQL(), c.WaitingJQL(), c.DoneJQL())
	}
}
