// Package poller fetches Jira state on an interval and emits immutable
// snapshots for the dashboard.
package poller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
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

	refresh chan struct{} // buffered 1; see Refresh

	mu         sync.Mutex
	me         string
	cols       []model.Column
	filterID   string // the board's saved filter; "" = do not scope
	colsAt     time.Time
	changelogs map[string]clEntry
	comments   map[string]cmEntry
	epics      map[string]epicEntry // by story key
	last       Snapshot
}

// epicEntry caches a story's epic for BoardRefreshInterval: a story rarely
// moves between epics, and the story's Updated is not in the subtask's data.
type epicEntry struct {
	at   time.Time
	epic jira.Parent
}

// cmEntry caches an issue's fetched comments, valid while the issue's
// Updated time is unchanged. A new comment bumps Updated.
type cmEntry struct {
	updated  time.Time
	comments []model.Comment
}

// clEntry caches when an issue entered its status, valid while the issue's
// Updated time is unchanged.
type clEntry struct {
	updated time.Time
	since   time.Time
}

// New returns a Poller for cfg that reads Jira through api.
func New(cfg *config.Config, api jira.API) *Poller {
	return &Poller{
		cfg: cfg, api: api, now: time.Now, after: time.After, changelogs: map[string]clEntry{}, comments: map[string]cmEntry{},
		epics: map[string]epicEntry{}, refresh: make(chan struct{}, 1),
	}
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
		bc, err := p.api.BoardConfig(ctx, p.cfg.Jira.BoardID)
		if err != nil {
			return Snapshot{}, err
		}
		p.cols, p.filterID, p.colsAt = bc.Columns, bc.FilterID, now
	}
	rules, err := p.cfg.Rules(p.me)
	if err != nil {
		return Snapshot{}, err
	}

	lanes := []struct {
		lane model.Lane
		jql  string
	}{
		{model.LaneMine, p.scoped(p.cfg.MineJQL())},
		{model.LaneWaiting, p.cfg.WaitingJQL()}, // global: waiting spans boards
		{model.LaneDone, p.scoped(p.cfg.DoneJQL())},
	}
	seen := map[string]bool{}
	stories := map[string]bool{} // stories whose subtasks were looked up
	var cards []model.Card
	for _, l := range lanes {
		issues, err := p.api.Search(ctx, l.jql, p.cfg.Jira.FlaggedField)
		if err != nil {
			return Snapshot{}, err
		}
		for _, iss := range issues {
			if seen[iss.Key] || p.cfg.IsMuted(iss.Key) {
				continue
			}
			if iss.ParentKey != "" {
				stories[iss.ParentKey] = true
				if err := p.rollUp(ctx, &iss, now); err != nil {
					return Snapshot{}, err
				}
			}
			if p.mutedParent(iss) {
				continue
			}
			seen[iss.Key] = true
			if iss.CommentsTruncated {
				if err := p.fillComments(ctx, &iss); err != nil {
					return Snapshot{}, err
				}
			}
			if l.lane != model.LaneDone {
				since, err := p.statusSince(ctx, iss)
				switch {
				case err == nil:
					iss.StatusSince = since
				case fatal(err):
					return Snapshot{}, err
				default:
					// One bad changelog degrades only its card; StatusSince
					// stays zero ("not yet known").
					iss.DecodeErrors = append(iss.DecodeErrors, fmt.Sprintf("changelog: %v", err))
				}
			}
			col := model.ColumnFor(p.cols, iss.StatusID)
			cards = append(cards, model.Evaluate(iss, col, l.lane, rules, now, false))
		}
	}
	// Drop cache entries for issues that left every lane (or were muted), so
	// the caches do not grow without bound over a long session.
	maps.DeleteFunc(p.changelogs, func(k string, _ clEntry) bool { return !seen[k] })
	maps.DeleteFunc(p.comments, func(k string, _ cmEntry) bool { return !seen[k] })
	maps.DeleteFunc(p.epics, func(k string, _ epicEntry) bool { return !stories[k] })
	return Snapshot{Columns: p.cols, Cards: cards, At: now}, nil
}

// mutedParent reports whether the issue's epic, or a subtask's story, is
// muted.
func (p *Poller) mutedParent(iss model.Issue) bool {
	return (iss.EpicKey != "" && p.cfg.IsMuted(iss.EpicKey)) ||
		(iss.ParentKey != "" && p.cfg.IsMuted(iss.ParentKey))
}

// rollUp moves a subtask from its story to the story's epic, or to no epic
// when the story has none. A non-fatal lookup failure leaves the subtask
// grouped under its story and marks the card with a decode error.
func (p *Poller) rollUp(ctx context.Context, iss *model.Issue, now time.Time) error {
	e, ok := p.epics[iss.ParentKey]
	if !ok || now.Sub(e.at) >= p.cfg.BoardRefreshInterval() {
		epic, err := p.api.ParentOf(ctx, iss.ParentKey)
		if err != nil {
			if fatal(err) {
				return err
			}
			iss.DecodeErrors = append(iss.DecodeErrors, fmt.Sprintf("epic: parent of %s: %v", iss.ParentKey, err))
			return nil
		}
		e = epicEntry{at: now, epic: epic}
		p.epics[iss.ParentKey] = e
	}
	iss.EpicKey, iss.EpicSummary = e.epic.Key, e.epic.Summary
	return nil
}

// scoped ANDs the board's saved filter onto a lane query, so the lane shows
// only this board's issues. A user's [jql] override is scoped too. With no
// filter ID the query is unchanged.
func (p *Poller) scoped(jql string) string {
	if p.filterID == "" {
		return jql
	}
	return "(" + jql + ") AND filter = " + p.filterID
}

// fatal reports whether a per-issue fetch error must fail the whole poll: a
// 401 (stop polling), a 429 (its RetryAfter must reach the snapshot), or a
// cancelled or expired context. Any other error only degrades that card.
func fatal(err error) bool {
	var ae *jira.APIError
	return jira.IsAuth(err) ||
		(errors.As(err, &ae) && ae.Status == http.StatusTooManyRequests) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// fillComments replaces a truncated first page of comments with the newest
// ones, fetched only when the issue changed since the last fetch. A non-fatal
// failure keeps the first page and marks the card with a decode error.
func (p *Poller) fillComments(ctx context.Context, iss *model.Issue) error {
	// A zero Updated (missing or undecodable) cannot tell a change apart, so
	// it is never cached.
	cacheable := !iss.Updated.IsZero()
	if e, ok := p.comments[iss.Key]; ok && cacheable && e.updated.Equal(iss.Updated) {
		iss.Comments = e.comments
		return nil
	}
	cms, err := p.api.Comments(ctx, iss.Key)
	if err != nil {
		if fatal(err) {
			return err
		}
		iss.DecodeErrors = append(iss.DecodeErrors, fmt.Sprintf("comments: truncated, fetch failed: %v", err))
		return nil
	}
	iss.Comments = cms
	if cacheable {
		p.comments[iss.Key] = cmEntry{updated: iss.Updated, comments: cms}
	}
	return nil
}

// statusSince reads the changelog only when the issue changed since the last
// read.
func (p *Poller) statusSince(ctx context.Context, iss model.Issue) (time.Time, error) {
	cacheable := !iss.Updated.IsZero() // see fillComments
	if e, ok := p.changelogs[iss.Key]; ok && cacheable && e.updated.Equal(iss.Updated) {
		return e.since, nil
	}
	ch, err := p.api.StatusChanges(ctx, iss.Key)
	if err != nil {
		return time.Time{}, err
	}
	since := model.StatusSince(iss.Created, iss.StatusID, ch)
	if cacheable {
		p.changelogs[iss.Key] = clEntry{updated: iss.Updated, since: since}
	}
	return since, nil
}

// Refresh asks a running Start loop to poll now. It never blocks: a request
// already pending absorbs this one, and a call after Start has stopped (a 401,
// or ctx done) does nothing.
func (p *Poller) Refresh() {
	select {
	case p.refresh <- struct{}{}:
	default:
	}
}

// Start polls now and then every PollInterval (or a 429's longer RetryAfter),
// sending each Snapshot on the returned channel. Refresh polls at once, or
// as soon as a 429's RetryAfter has passed.
// Polling stops, and the channel closes, when ctx is done or a poll gets a
// 401.
func (p *Poller) Start(ctx context.Context) <-chan Snapshot {
	out := make(chan Snapshot, 1)
	go func() {
		defer close(out)
		for {
			snap := p.PollOnce(ctx)
			if ctx.Err() != nil {
				return // the poll was cut short; its error is not news
			}
			select {
			case out <- snap:
			case <-ctx.Done():
				return
			}
			if snap.AuthFailed {
				return
			}
			if !p.wait(ctx, snap.RetryAfter) {
				return
			}
		}
	}()
	return out
}

// wait blocks until the next poll is due: PollInterval (or retryAfter, if
// longer) from now, or a Refresh that is not inside the retryAfter window.
// A Refresh inside the window waits out the rest of it. wait reports false
// when ctx is done.
func (p *Poller) wait(ctx context.Context, retryAfter time.Duration) bool {
	notBefore := p.now().Add(retryAfter)
	timer := p.after(max(p.cfg.PollInterval(), retryAfter))
	for {
		select {
		case <-timer:
			return true
		case <-p.refresh:
			left := notBefore.Sub(p.now())
			if left <= 0 {
				return true
			}
			timer = p.after(left)
		case <-ctx.Done():
			return false
		}
	}
}
