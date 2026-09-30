package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/config"
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
	c, err := config.Load(write(t, minimal))
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
	if c.MineJQL() != config.DefaultMineJQL || c.WaitingJQL() != config.DefaultWaitingJQL || c.DoneJQL() != config.DefaultDoneJQL {
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
	cases := []struct {
		name, body, want string
	}{
		{"no site", "[jira]\n  email = \"me@example.com\"\n  board_id = 1\n", "jira.site is required"},
		{"no board", "[jira]\n  site = \"https://example.atlassian.net\"\n  email = \"me@example.com\"\n", "jira.board_id is required"},
		{"bad view", minimal + "\n[settings]\n  default_view = \"grid\"\n", "settings.default_view"},
		{"bad stuck", minimal + "\n[settings]\n  stuck_alert_after = \"later\"\n", "settings.stuck_alert_after"},
		{"bad age", minimal + "\n[thresholds.\"UA\"]\n  yellow = \"soon\"\n", `thresholds."UA".yellow`},
	}
	for _, c := range cases {
		_, err := config.Load(write(t, c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
}

func TestResolveTokenOrder(t *testing.T) {
	t.Setenv("JG_TEST_TOKEN", "from-env")
	t.Setenv("JIRA_API_TOKEN", "fallback")
	if tok, err := config.ResolveToken(config.Jira{TokenCommand: "printf from-cmd", TokenEnv: "JG_TEST_TOKEN"}); err != nil || tok != "from-cmd" {
		t.Errorf("cmd first, got %q, %v", tok, err)
	}
	if tok, err := config.ResolveToken(config.Jira{TokenEnv: "JG_TEST_TOKEN"}); err != nil || tok != "from-env" {
		t.Errorf("env second, got %q, %v", tok, err)
	}
	if tok, err := config.ResolveToken(config.Jira{}); err != nil || tok != "fallback" {
		t.Errorf("JIRA_API_TOKEN last, got %q, %v", tok, err)
	}
	t.Setenv("JIRA_API_TOKEN", "")
	if _, err := config.ResolveToken(config.Jira{}); err == nil {
		t.Error("want error with no token source")
	}
}

func TestResolveTokenCommandErrors(t *testing.T) {
	t.Setenv("JG_TEST_TOKEN", "from-env")
	for name, cmd := range map[string]string{
		"fails":     "echo nope >&2; exit 3",
		"no output": "true",
	} {
		if _, err := config.ResolveToken(config.Jira{TokenCommand: cmd, TokenEnv: "JG_TEST_TOKEN"}); err == nil {
			t.Errorf("%s: want error, not a fall-through", name)
		}
	}
}

func TestMuteRoundTrip(t *testing.T) {
	c, err := config.Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	c2, err := config.Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !c2.IsMuted("ABC-9") || c2.IsMuted("ABC-1") {
		t.Errorf("mutes %v", c2.Muted)
	}
}

func TestUnmuteRoundTrip(t *testing.T) {
	c, err := config.Load(write(t, "muted = [\"ABC-9\", \"ABC-2\"]\n"+minimal))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-9", false); err != nil {
		t.Fatal(err)
	}
	c2, err := config.Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if c2.IsMuted("ABC-9") || !c2.IsMuted("ABC-2") {
		t.Errorf("mutes %v", c2.Muted)
	}
}

func TestLoadTrimsSiteSlash(t *testing.T) {
	c, err := config.Load(write(t, "[jira]\n  site = \"https://example.atlassian.net/\"\n  email = \"me@example.com\"\n  board_id = 7\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Jira.Site != "https://example.atlassian.net" {
		t.Errorf("site %q", c.Jira.Site)
	}
}

func TestSaveKeepsWebhooks(t *testing.T) {
	body := minimal + "\n[[webhooks]]\n  url = \"https://hooks.example.com/x\"\n  secret = \"s\"\n"
	c, err := config.Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	c2, err := config.Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	if len(c2.Webhooks) != 1 || c2.Webhooks[0].URL != "https://hooks.example.com/x" || c2.Webhooks[0].Secret != "s" {
		t.Errorf("webhooks %+v", c2.Webhooks)
	}
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p, err := config.DefaultPath(); err != nil || p != filepath.Join("/xdg", "jira-green", "config.toml") {
		t.Errorf("xdg path %q, %v", p, err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/u")
	if p, err := config.DefaultPath(); err != nil || p != filepath.Join("/home/u", ".config", "jira-green", "config.toml") {
		t.Errorf("home path %q, %v", p, err)
	}
}

func TestDefaultPathNoHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if p, err := config.DefaultPath(); err == nil {
		t.Errorf("want error with no home directory, got %q", p)
	}
}

func TestWriteStarter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	j := config.Jira{Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JG_TEST_TOKEN", BoardID: 7}
	if _, err := config.WriteStarter(p, j); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Jira != j || c.DefaultView() != "kanban" {
		t.Errorf("starter %+v", c)
	}
	if r, err := c.Rules(""); err != nil || r.Thresholds["In Progress"].Red != 5*24*time.Hour {
		t.Errorf("starter rules %+v, %v", r, err)
	}
	if _, err := config.WriteStarter(p, j); err == nil {
		t.Error("want error: starter must not overwrite")
	}
}

func TestThresholdsMergeKeepsOtherDefaults(t *testing.T) {
	c, err := config.Load(write(t, minimal+"\n[thresholds.\"In Progress\"]\n  yellow = \"4d\"\n  red = \"6d\"\n"))
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
	c, err := config.Load(write(t, minimal+"\n[thresholds.\"Code Review\"]\n  yellow = \"4h\"\n"))
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
	c, err := config.Load(write(t, minimal+"\n[thresholds.\"UA\"]\n"))
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
	c, err := config.Load(write(t, minimal+"\n[thresholds.\"QA\"]\n  red = \"3d\"\n"))
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
	c, err := config.Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	want, err := c.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	c2, err := config.Load(c.Path())
	if err != nil {
		t.Fatal(err)
	}
	got, err := c2.Rules("")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("after save got %+v, want %+v", got.Thresholds, want.Thresholds)
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	_, err := config.Load(write(t, minimal+"  token_cmd = \"pass jira\"\n"))
	if err == nil || !strings.Contains(err.Error(), "jira.token_cmd") {
		t.Errorf("want unknown-key error naming jira.token_cmd, got %v", err)
	}
}

func TestLoadRejectsMisplacedMuted(t *testing.T) {
	_, err := config.Load(write(t, minimal+"muted = [\"ABC-9\"]\n"))
	if err == nil || !strings.Contains(err.Error(), "jira.muted") {
		t.Errorf("want unknown-key error naming jira.muted, got %v", err)
	}
}

func TestSaveKeepsOnlyWhatUserWrote(t *testing.T) {
	c, err := config.Load(write(t, minimal))
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
	c, err := config.Load(write(t, "blocked_labels = []\n"+minimal))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-9", true); err != nil {
		t.Fatal(err)
	}
	c2, err := config.Load(c.Path())
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
	c, err := config.Load(write(t, body))
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

func TestSaveTightensMode(t *testing.T) {
	p := write(t, minimal)
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if m := fi.Mode().Perm(); m != 0o600 {
		t.Errorf("mode %o, want 600", m)
	}
}

func TestLoadValidation(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"negative poll", minimal + "\n[settings]\n  poll_interval_seconds = -1\n", "settings.poll_interval_seconds"},
		{"negative board refresh", minimal + "\n[settings]\n  board_refresh_interval_seconds = -5\n", "settings.board_refresh_interval_seconds"},
		{"no email", "[jira]\n  site = \"https://example.atlassian.net\"\n  board_id = 7\n", "jira.email"},
		{"http site", "[jira]\n  site = \"http://example.atlassian.net\"\n  email = \"me@example.com\"\n  board_id = 7\n", "jira.site"},
		{"hostless site", "[jira]\n  site = \"example.atlassian.net\"\n  email = \"me@example.com\"\n  board_id = 7\n", "jira.site"},
		{"yellow not below red", minimal + "\n[thresholds.\"UA\"]\n  yellow = \"2d\"\n  red = \"1d\"\n", "yellow must be less than red"},
		{"yellow equals red", minimal + "\n[thresholds.\"UA\"]\n  yellow = \"1d\"\n  red = \"24h\"\n", "yellow must be less than red"},
		{"empty webhook url", minimal + "\n[[webhooks]]\n  url = \"https://hooks.example.com/a\"\n[[webhooks]]\n  secret = \"s\"\n", "webhooks[1].url"},
		{"non-http webhook", minimal + "\n[[webhooks]]\n  url = \"ftp://hooks.example.com/a\"\n", "webhooks[0].url"},
		{"hostless webhook", minimal + "\n[[webhooks]]\n  url = \"hooks.example.com/a\"\n", "webhooks[0].url"},
	}
	for _, c := range cases {
		_, err := config.Load(write(t, c.body))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
}

func TestLoadAcceptsHTTPWebhook(t *testing.T) {
	if _, err := config.Load(write(t, minimal+"\n[[webhooks]]\n  url = \"http://localhost:8080/hook\"\n")); err != nil {
		t.Errorf("plain http webhook rejected: %v", err)
	}
}

func TestLoadValidationReportsFirstColumnInOrder(t *testing.T) {
	body := minimal + "\n[thresholds.\"Zeta\"]\n  yellow = \"soon\"\n[thresholds.\"Alpha\"]\n  yellow = \"soon\"\n[thresholds.\"Mid\"]\n  red = \"later\"\n"
	for range 20 {
		_, err := config.Load(write(t, body))
		if err == nil || !strings.Contains(err.Error(), `"Alpha"`) {
			t.Fatalf("want the error for the first column in sorted order (Alpha), got %v", err)
		}
	}
}

func TestLoadRejectsLiteralToken(t *testing.T) {
	_, err := config.Load(write(t, minimal+"  token = \"secret\"\n"))
	if err == nil || !strings.Contains(err.Error(), "jira.token") {
		t.Errorf("want unknown-key error naming jira.token, got %v", err)
	}
}

func TestResolveTokenEnvUnsetIsAnError(t *testing.T) {
	t.Setenv("JIRA_API_TOKEN", "fallback")
	for _, v := range []string{"", "   "} {
		t.Setenv("JG_UNSET_TOKEN", v)
		tok, err := config.ResolveToken(config.Jira{TokenEnv: "JG_UNSET_TOKEN"})
		if err == nil || !strings.Contains(err.Error(), `token_env "JG_UNSET_TOKEN" is unset`) {
			t.Errorf("value %q: want token_env unset error, got token %q, err %v", v, tok, err)
		}
	}
}

func TestResolveTokenCommandTimesOut(t *testing.T) {
	t.Cleanup(config.SetTokenCommandTimeout(100 * time.Millisecond))
	start := time.Now()
	_, err := config.ResolveToken(config.Jira{TokenCommand: "sleep 5; echo late"})
	if err == nil {
		t.Fatal("want timeout error")
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("took %v, want it cut off by the timeout", el)
	}
}

func TestWriteStarterExistingFile(t *testing.T) {
	p := write(t, minimal)
	j := config.Jira{Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JG_TEST_TOKEN", BoardID: 7}
	_, err := config.WriteStarter(p, j)
	if err == nil || !strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "--force") {
		t.Errorf("want plain already-exists error, got %v", err)
	}
}

func TestWriteStarterStatError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	p := filepath.Join(dir, "config.toml")
	j := config.Jira{Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JG_TEST_TOKEN", BoardID: 7}
	_, err := config.WriteStarter(p, j)
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Errorf("want an error naming %s from the existence check, got %v", p, err)
	}
}

func TestBacklogJQLDefaultAndOverride(t *testing.T) {
	c, err := config.Load(write(t, minimal))
	if err != nil {
		t.Fatal(err)
	}
	const def = `assignee = currentUser() AND statusCategory != Done AND (sprint IS EMPTY OR sprint NOT IN openSprints())`
	if c.BacklogJQL() != def || config.DefaultBacklogJQL != def {
		t.Errorf("default backlog JQL %q", c.BacklogJQL())
	}
	c, err = config.Load(write(t, minimal+"[jql]\n  backlog = \"project = ABC AND sprint IS EMPTY\"\n"))
	if err != nil {
		t.Fatal(err) // [jql] backlog must be a known key under strict decoding
	}
	if c.BacklogJQL() != "project = ABC AND sprint IS EMPTY" {
		t.Errorf("backlog override %q", c.BacklogJQL())
	}
}

func TestCreateLeavesAnExistingFileAlone(t *testing.T) {
	p := write(t, "old")
	c, err := config.New(p, config.Jira{Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JG_TEST_TOKEN", BoardID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Create(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("want an already-exists error, got %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Errorf("file changed: %q", b)
	}
	if ents, _ := os.ReadDir(filepath.Dir(p)); len(ents) != 1 {
		t.Errorf("temp file left behind: %v", ents)
	}
}

func TestNewRequiresExactlyOneTokenSource(t *testing.T) {
	base := config.Jira{Site: "https://example.atlassian.net", Email: "me@example.com", BoardID: 7}
	both, neither := base, base
	both.TokenEnv, both.TokenCommand = "JG_TEST_TOKEN", "echo tok"
	for name, j := range map[string]config.Jira{"both": both, "neither": neither} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "config.toml")
			if _, err := config.New(p, j); err == nil || !strings.Contains(err.Error(), "exactly one") {
				t.Errorf("New: want an exactly-one error, got %v", err)
			}
			if _, err := config.WriteStarter(p, j); err == nil {
				t.Error("WriteStarter: want error")
			}
			if _, err := os.Stat(p); !os.IsNotExist(err) {
				t.Errorf("config written: %v", err)
			}
		})
	}
}

func TestSaveKeepsASymlinkedConfig(t *testing.T) {
	target := write(t, minimal) // e.g. a file in Dropbox
	link := filepath.Join(t.TempDir(), "config.toml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SetMuted("ABC-1", true); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config.toml is no longer a symlink: %v %v", fi.Mode(), err)
	}
	if b, _ := os.ReadFile(target); !strings.Contains(string(b), "ABC-1") {
		t.Errorf("the target lacks the new content:\n%s", b)
	}
}

func TestWriteStarterReturnsNoConfigOnError(t *testing.T) {
	j := config.Jira{Site: "https://example.atlassian.net", Email: "me@example.com", TokenEnv: "JG_TEST_TOKEN", BoardID: 7}
	if c, err := config.WriteStarter(write(t, minimal), j); err == nil || c != nil {
		t.Errorf("onto an existing file: config %v, err %v; want nil and an error", c, err)
	}
}

func TestSetMutedFailedSaveLeavesMutesUnchanged(t *testing.T) {
	c, err := config.Load(write(t, "muted = [\"ABC-1\", \"ABC-2\"]\n"+minimal))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(c.Path())
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if err := c.SetMuted("ABC-9", true); err == nil {
		t.Fatal("save into a read-only dir succeeded")
	}
	if err := c.SetMuted("ABC-1", false); err == nil {
		t.Fatal("save into a read-only dir succeeded")
	}
	if c.IsMuted("ABC-9") || !c.IsMuted("ABC-1") || !c.IsMuted("ABC-2") {
		t.Errorf("mutes changed after failed saves: %v", c.MutedKeys())
	}
}
