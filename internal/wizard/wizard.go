// Package wizard runs the interactive `jira-green init` form.
package wizard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
)

// API is the part of the Jira client Finish needs.
type API interface {
	Myself(ctx context.Context) (jira.User, error)
	FindFieldID(ctx context.Context, name string) (string, error)
}

// Answers is what the form collected. Exactly one of TokenCommand and
// TokenEnv is set.
type Answers struct {
	Site         string
	Email        string
	TokenCommand string
	TokenEnv     string
	BoardID      int
}

// Finish is the non-interactive end of the wizard: it verifies the account,
// looks up the Flagged field, and writes the config to path.
func Finish(ctx context.Context, api API, a Answers, path string, force bool) (*config.Config, error) {
	if err := validSite(a.Site); err != nil {
		return nil, err
	}
	if _, err := api.Myself(ctx); err != nil {
		return nil, fmt.Errorf("verify account: %w", err)
	}
	flagged, err := api.FindFieldID(ctx, "Flagged")
	if err != nil {
		return nil, err
	}
	if force {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return config.WriteStarter(path, config.Jira{
		Site:         a.Site,
		Email:        a.Email,
		TokenCommand: a.TokenCommand,
		TokenEnv:     a.TokenEnv,
		BoardID:      a.BoardID,
		FlaggedField: flagged,
	})
}

// validSite checks the site before anything is removed or fetched;
// config.WriteStarter checks it again.
func validSite(s string) error {
	if u, err := url.Parse(s); err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("site must be an https URL such as https://example.atlassian.net, got %q", s)
	}
	return nil
}
