// Package config loads and saves ~/.config/jira-green/config.toml.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Settings is the optional [settings] table. Zero values mean "use the
// default"; read them through the Config accessors.
type Settings struct {
	PollIntervalSeconds         int    `toml:"poll_interval_seconds,omitempty"`
	BoardRefreshIntervalSeconds int    `toml:"board_refresh_interval_seconds,omitempty"`
	StuckAlertAfter             string `toml:"stuck_alert_after,omitempty"`
	DefaultView                 string `toml:"default_view,omitempty"` // "kanban" | "list"
}

// Jira is the [jira] table: the site, account, token source, and board.
type Jira struct {
	Site         string `toml:"site"`
	Email        string `toml:"email,omitempty"`
	TokenEnv     string `toml:"token_env,omitempty"`
	TokenCommand string `toml:"token_command,omitempty"`
	BoardID      int    `toml:"board_id,omitempty"`
	FlaggedField string `toml:"flagged_field,omitempty"` // e.g. customfield_10021; found by init
}

// JQL is the optional [jql] table of lane queries. Read it through
// MineJQL, WaitingJQL, BacklogJQL, and DoneJQL.
type JQL struct {
	Mine    string `toml:"mine,omitempty"`
	Waiting string `toml:"waiting,omitempty"`
	Backlog string `toml:"backlog,omitempty"`
	Done    string `toml:"done,omitempty"`
}

// Age is one [thresholds."Column"] table: ages written like "3d" or "12h".
// A table with neither level disables that column.
type Age struct {
	Yellow string `toml:"yellow,omitempty"`
	Red    string `toml:"red,omitempty"`
}

// Webhook is one [[webhooks]] entry that receives stuck-card alerts.
type Webhook struct {
	URL    string `toml:"url"`
	Secret string `toml:"secret,omitempty"`
}

// Config is config.toml as the user wrote it. Defaults are not stored in
// it; they are resolved by the accessor methods.
//
// Muted is the only field that changes after Load: SetMuted (the UI) and
// IsMuted (the poller goroutine) guard it with mu, so read it through
// IsMuted or MutedKeys, never directly. Every other field is read-only once
// Load returns, so Rules and the other accessors need no lock.
type Config struct {
	Settings      Settings       `toml:"settings,omitempty"`
	Jira          Jira           `toml:"jira"`
	JQL           JQL            `toml:"jql,omitempty"`
	Thresholds    map[string]Age `toml:"thresholds,omitempty"`
	BlockedLabels []string       `toml:"blocked_labels"`  // nil = default; [] = none
	Muted         []string       `toml:"muted,omitempty"` // issue or epic keys
	Webhooks      []Webhook      `toml:"webhooks,omitempty"`

	path string
	mu   sync.RWMutex // guards Muted
}

// Default lane queries, used when [jql] leaves a key unset.
const (
	DefaultMineJQL    = `assignee = currentUser() AND sprint IN openSprints() AND statusCategory != Done`
	DefaultWaitingJQL = `(reporter = currentUser() OR watcher = currentUser()) AND assignee != currentUser() AND statusCategory != Done`
	DefaultBacklogJQL = `assignee = currentUser() AND statusCategory != Done AND (sprint IS EMPTY OR sprint NOT IN openSprints())`
	DefaultDoneJQL    = `assignee = currentUser() AND sprint IN openSprints() AND statusCategory = Done`
)

func defaultThresholds() map[string]Age {
	return map[string]Age{
		"In Progress": {Yellow: "3d", Red: "5d"},
		"Code Review": {Yellow: "1d", Red: "2d"},
		"UA":          {Yellow: "1d", Red: "2d"},
	}
}

// DefaultPath is $XDG_CONFIG_HOME/jira-green/config.toml, or
// ~/.config/jira-green/config.toml when XDG_CONFIG_HOME is unset. It returns
// an error when neither is available rather than guessing a relative path.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find config path: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "jira-green", "config.toml"), nil
}

// Load reads and validates the config at path. Unknown keys are an error.
func Load(path string) (*Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, len(undecoded))
		for i, k := range undecoded {
			keys[i] = k.String()
		}
		return nil, fmt.Errorf("%s: unknown key(s): %s", path, strings.Join(keys, ", "))
	}
	c.path = path
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// validate checks the values the user wrote. It writes nothing back except
// normalising jira.site, so Save persists only what the user wrote; defaults
// are resolved by the accessors.
func (c *Config) validate() error {
	if c.Settings.PollIntervalSeconds < 0 {
		return fmt.Errorf("settings.poll_interval_seconds must be >= 0 (0 = default), got %d", c.Settings.PollIntervalSeconds)
	}
	if c.Settings.BoardRefreshIntervalSeconds < 0 {
		return fmt.Errorf("settings.board_refresh_interval_seconds must be >= 0 (0 = default), got %d", c.Settings.BoardRefreshIntervalSeconds)
	}
	if v := c.DefaultView(); v != "kanban" && v != "list" {
		return fmt.Errorf("settings.default_view must be kanban or list, got %q", v)
	}
	if c.Settings.StuckAlertAfter != "" {
		if _, err := model.ParseAge(c.Settings.StuckAlertAfter); err != nil {
			return fmt.Errorf("settings.stuck_alert_after: %w", err)
		}
	}
	if strings.TrimSpace(c.Jira.Site) == "" {
		return errors.New("jira.site is required")
	}
	c.Jira.Site = strings.TrimRight(c.Jira.Site, "/")
	if u, err := url.Parse(c.Jira.Site); err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("jira.site must be an https URL such as https://example.atlassian.net, got %q", c.Jira.Site)
	}
	if strings.TrimSpace(c.Jira.Email) == "" {
		return errors.New("jira.email is required")
	}
	if c.Jira.BoardID <= 0 {
		return errors.New("jira.board_id is required")
	}
	_, err := c.Rules("")
	return err
}

// MineJQL is the query for the Mine lane.
func (c *Config) MineJQL() string { return orDefault(c.JQL.Mine, DefaultMineJQL) }

// WaitingJQL is the query for the Waiting on others lane.
func (c *Config) WaitingJQL() string { return orDefault(c.JQL.Waiting, DefaultWaitingJQL) }

// BacklogJQL is the query for the Backlog lane: my open issues outside the
// open sprints.
func (c *Config) BacklogJQL() string { return orDefault(c.JQL.Backlog, DefaultBacklogJQL) }

// DoneJQL is the query for the Done this sprint lane.
func (c *Config) DoneJQL() string { return orDefault(c.JQL.Done, DefaultDoneJQL) }

// PollInterval is how often issues are polled. Default 60s.
func (c *Config) PollInterval() time.Duration {
	return secondsOr(c.Settings.PollIntervalSeconds, 60)
}

// BoardRefreshInterval is how often the board configuration is re-read.
// Default 10m.
func (c *Config) BoardRefreshInterval() time.Duration {
	return secondsOr(c.Settings.BoardRefreshIntervalSeconds, 600)
}

// StuckAlertAfter is how long a card must stay red before webhooks fire.
// Default 2h. Load has already validated the value.
func (c *Config) StuckAlertAfter() time.Duration {
	d, err := model.ParseAge(orDefault(c.Settings.StuckAlertAfter, "2h"))
	if err != nil {
		return 2 * time.Hour
	}
	return d
}

// DefaultView is "kanban" or "list". Default kanban.
func (c *Config) DefaultView() string { return orDefault(c.Settings.DefaultView, "kanban") }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func secondsOr(n, def int) time.Duration {
	if n <= 0 {
		n = def
	}
	return time.Duration(n) * time.Second
}

// Rules builds the health rules for the given account ID. Threshold defaults
// apply per column: a user [thresholds.X] table replaces only column X, and a
// table with neither yellow nor red disables that column. BlockedLabels
// defaults to ["blocked"] only when the key is absent.
func (c *Config) Rules(me string) (model.Rules, error) {
	labels := c.BlockedLabels
	if labels == nil {
		labels = []string{"blocked"}
	}
	merged := defaultThresholds()
	for col, a := range c.Thresholds {
		merged[col] = a
	}
	r := model.Rules{Me: me, BlockedLabels: labels, Thresholds: map[string]model.Threshold{}}
	cols := slices.Sorted(maps.Keys(merged)) // deterministic error order
	for _, col := range cols {
		a := merged[col]
		var th model.Threshold
		var err error
		if a.Yellow != "" {
			if th.Yellow, err = model.ParseAge(a.Yellow); err != nil {
				return r, fmt.Errorf("thresholds.%q.yellow: %w", col, err)
			}
		}
		if a.Red != "" {
			if th.Red, err = model.ParseAge(a.Red); err != nil {
				return r, fmt.Errorf("thresholds.%q.red: %w", col, err)
			}
		}
		if th.Yellow > 0 && th.Red > 0 && th.Yellow >= th.Red {
			return r, fmt.Errorf("thresholds.%q: yellow must be less than red (%s >= %s)", col, a.Yellow, a.Red)
		}
		r.Thresholds[col] = th
	}
	return r, nil
}

// Path is the file the config was loaded from and saves to.
func (c *Config) Path() string { return c.path }

// IsMuted reports whether an issue or epic key is muted. It is safe to call
// while another goroutine calls SetMuted.
func (c *Config) IsMuted(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Contains(c.Muted, key)
}

// MutedKeys returns a copy of the mute list. It is safe to call while
// another goroutine calls SetMuted.
func (c *Config) MutedKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return slices.Clone(c.Muted)
}

// SetMuted adds or removes a key from the mute list and saves. It holds the
// write lock through the save, so concurrent calls save in order.
func (c *Config) SetMuted(key string, muted bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	has := slices.Contains(c.Muted, key)
	switch {
	case muted && !has:
		c.Muted = append(c.Muted, key)
	case !muted && has:
		c.Muted = slices.DeleteFunc(c.Muted, func(k string) bool { return k == key })
	default:
		return nil
	}
	return c.save()
}

// Save writes the config atomically: a temp file in the same directory,
// mode 0600, renamed over the target.
func (c *Config) Save() error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.save()
}

// save is Save for a caller that already holds mu.
func (c *Config) save() error {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return err
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".config-*.toml")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = os.Remove(tmp) }() // no-op after a successful rename
	if _, err := f.Write(buf.Bytes()); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// tokenCommandTimeout bounds how long token_command may run.
var tokenCommandTimeout = 10 * time.Second

// ResolveToken returns the API token: token_command, then token_env, then
// $JIRA_API_TOKEN (the variable go-jira-cli also reads) only when neither is
// configured. A configured source that fails is an error, never a fall-through.
// The config file never holds the token itself.
func ResolveToken(j Jira) (string, error) {
	if j.TokenCommand != "" {
		ctx, cancel := context.WithTimeout(context.Background(), tokenCommandTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "sh", "-c", j.TokenCommand)
		cmd.WaitDelay = time.Second // don't wait on a killed shell's children holding stdout
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if ctx.Err() != nil {
			return "", fmt.Errorf("token_command timed out after %v", tokenCommandTimeout)
		}
		if err != nil {
			return "", fmt.Errorf("token_command failed: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		tok := strings.TrimSpace(string(out))
		if tok == "" {
			return "", errors.New("token_command produced no output")
		}
		return tok, nil
	}
	if j.TokenEnv != "" {
		tok := strings.TrimSpace(os.Getenv(j.TokenEnv))
		if tok == "" {
			return "", fmt.Errorf("token_env %q is unset", j.TokenEnv)
		}
		return tok, nil
	}
	if tok := strings.TrimSpace(os.Getenv("JIRA_API_TOKEN")); tok != "" {
		return tok, nil
	}
	return "", errors.New("no token: set jira.token_command, jira.token_env, or JIRA_API_TOKEN")
}

// WriteStarter writes a new config for the wizard. It refuses to overwrite.
func WriteStarter(path string, j Jira) (*Config, error) {
	switch _, err := os.Stat(path); {
	case err == nil:
		return nil, fmt.Errorf("%s already exists", path)
	case !errors.Is(err, fs.ErrNotExist):
		return nil, fmt.Errorf("check %s: %w", path, err)
	}
	c, err := NewStarter(path, j)
	if err != nil {
		return nil, err
	}
	return c, c.Save()
}

// NewStarter validates j and returns a Config that saves to path. It writes
// nothing; Save replaces any existing file atomically.
func NewStarter(path string, j Jira) (*Config, error) {
	c := &Config{Jira: j, path: path}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c, nil
}
