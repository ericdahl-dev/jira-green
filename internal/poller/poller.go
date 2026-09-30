// Package poller fetches Jira state on an interval and emits immutable
// snapshots for the dashboard.
package poller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ericdahl-dev/jira-green/internal/config"
	"github.com/ericdahl-dev/jira-green/internal/jira"
	"github.com/ericdahl-dev/jira-green/internal/model"
)

// Snapshot is one immutable poll result.
type Snapshot struct {
	Columns []model.Column
	Cards   []model.Card
	At      time.Time
	Err     error
	// AuthFailed is set on a 401: the credentials are bad and polling stops.
	AuthFailed bool
	// RetryAfter is the server's requested backoff after a 429; Start waits
	// at least this long before the next poll.
	RetryAfter time.Duration
}

// Poller turns Jira queries into Snapshots.
type Poller struct {
	cfg   *config.Config
	api   jira.API
	now   func() time.Time
	after func(time.Duration) <-chan time.Time // Start's timer between polls

	mu         sync.Mutex
	me         string
	cols       []model.Column
	colsAt     time.Time
	changelogs map[string]clEntry
	last       Snapshot
}

// clEntry caches when an issue entered its status, valid while the issue's
// Updated time is unchanged.
type clEntry struct {
	updated time.Time
	since   time.Time
}

// New returns a Poller for cfg that reads Jira through api.
func New(cfg *config.Config, api jira.API) *Poller {
	return &Poller{cfg: cfg, api: api, now: time.Now, after: time.After, changelogs: map[string]clEntry{}}
}

// PollOnce performs one poll synchronously. It is safe to call from any
// goroutine.
func (p *Poller) PollOnce(ctx context.Context) Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	snap, err := p.fetch(ctx, now)
	if err != nil {
		snap = p.staleFrom(err)
		snap.AuthFailed = jira.IsAuth(err)
		var ae *jira.APIError
		if errors.As(err, &ae) {
			snap.RetryAfter = ae.RetryAfter
		}
	}
	p.last = snap
	return snap
}

// staleFrom keeps the last good cards, columns, and time, and marks the
// cards stale. Worst keeps a red or yellow light, so a network blip does not
// hide a problem.
func (p *Poller) staleFrom(err error) Snapshot {
	s := Snapshot{Columns: p.last.Columns, At: p.last.At, Err: err}
	for _, c := range p.last.Cards {
		c.Light = model.Worst(c.Light, model.Stale)
		s.Cards = append(s.Cards, c)
	}
	return s
}

func (p *Poller) fetch(ctx context.Context, now time.Time) (Snapshot, error) {
	if p.me == "" {
		u, err := p.api.Myself(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		p.me = u.AccountID
	}
	if p.cols == nil || now.Sub(p.colsAt) >= p.cfg.BoardRefreshInterval() {
		cols, err := p.api.BoardColumns(ctx, p.cfg.Jira.BoardID)
		if err != nil {
			return Snapshot{}, err
		}
		p.cols, p.colsAt = cols, now
	}
	rules, err := p.cfg.Rules(p.me)
	if err != nil {
		return Snapshot{}, err
	}

	lanes := []struct {
		lane model.Lane
		jql  string
	}{
		{model.LaneMine, p.cfg.MineJQL()},
		{model.LaneWaiting, p.cfg.WaitingJQL()},
		{model.LaneDone, p.cfg.DoneJQL()},
	}
	seen := map[string]bool{}
	var cards []model.Card
	for _, l := range lanes {
		issues, err := p.api.Search(ctx, l.jql, p.cfg.Jira.FlaggedField)
		if err != nil {
			return Snapshot{}, err
		}
		for _, iss := range issues {
			if seen[iss.Key] || p.cfg.IsMuted(iss.Key) || (iss.EpicKey != "" && p.cfg.IsMuted(iss.EpicKey)) {
				continue
			}
			seen[iss.Key] = true
			if l.lane != model.LaneDone {
				since, err := p.statusSince(ctx, iss)
				if err != nil {
					return Snapshot{}, err
				}
				iss.StatusSince = since
			}
			col := model.ColumnFor(p.cols, iss.StatusID)
			cards = append(cards, model.Evaluate(iss, col, l.lane, rules, now, false))
		}
	}
	return Snapshot{Columns: p.cols, Cards: cards, At: now}, nil
}

// statusSince reads the changelog only when the issue changed since the last
// read.
func (p *Poller) statusSince(ctx context.Context, iss model.Issue) (time.Time, error) {
	if e, ok := p.changelogs[iss.Key]; ok && e.updated.Equal(iss.Updated) {
		return e.since, nil
	}
	ch, err := p.api.StatusChanges(ctx, iss.Key)
	if err != nil {
		return time.Time{}, err
	}
	since := model.StatusSince(iss.Created, iss.StatusID, ch)
	p.changelogs[iss.Key] = clEntry{updated: iss.Updated, since: since}
	return since, nil
}

// Start polls now and then every PollInterval (or a 429's longer
// RetryAfter) until ctx is done or a poll fails auth, sending each Snapshot on the returned channel, which closes
// when polling stops. A send on the returned refresh channel polls at once.
func (p *Poller) Start(ctx context.Context) (<-chan Snapshot, chan<- struct{}) {
	out := make(chan Snapshot, 1)
	refresh := make(chan struct{}, 1)
	go func() {
		defer close(out)
		for {
			snap := p.PollOnce(ctx)
			select {
			case out <- snap:
			case <-ctx.Done():
				return
			}
			if snap.AuthFailed {
				return
			}
			select {
			case <-p.after(max(p.cfg.PollInterval(), snap.RetryAfter)):
			case <-refresh:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, refresh
}
