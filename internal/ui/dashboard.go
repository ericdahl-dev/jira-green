package ui

import (
	"fmt"
	"maps"
	"os/exec"
	"runtime"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ericdahl-dev/jira-green/internal/model"
	"github.com/ericdahl-dev/jira-green/internal/poller"
)

// ViewMode is the dashboard's chosen board view.
type ViewMode int

// The board views.
const (
	ViewKanban ViewMode = iota
	ViewList
)

// RefreshMsg asks main to poll now (poller.Refresh).
type RefreshMsg struct{}

// OpenManageMsg asks main to show the manage screen. Groups are every card
// of the latest snapshot, Done included, by epic.
type OpenManageMsg struct{ Groups []model.EpicGroup }

// Dashboard is the main screen. It performs no I/O: anything slow goes back
// to main as a tea.Cmd message.
type Dashboard struct {
	mode      ViewMode
	snap      poller.Snapshot
	board     model.Board
	groups    []model.EpicGroup
	cur       Cursor
	listSel   int
	collapsed map[string]bool
	showDone  bool
	showHelp  bool
	detail    *model.Card // shown instead of the board when set
	picker    *Picker     // shown over the board or detail when set
	width     int
	height    int
	openURL   func(string) error
}

// NewDashboard starts in defaultView ("kanban" or "list"). openURL opens a
// card in the browser; main passes OpenBrowser, tests a fake.
func NewDashboard(defaultView string, openURL func(string) error) Dashboard {
	// 80 columns until the first WindowSizeMsg says otherwise.
	d := Dashboard{openURL: openURL, collapsed: map[string]bool{}, width: 80}
	if defaultView == "list" {
		d.mode = ViewList
	}
	return d
}

// Mode is the chosen view. A terminal too narrow for kanban shows the list
// without changing it.
func (d Dashboard) Mode() ViewMode { return d.mode }

// Init satisfies tea.Model.
func (d Dashboard) Init() tea.Cmd { return nil }

// Update handles one message.
func (d Dashboard) Update(msg tea.Msg) (Dashboard, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		d.width, d.height = msg.Width, msg.Height
	case poller.Snapshot:
		d.snap = msg
		d.relayout()
		d.refreshDetail()
	case TransitionsLoadedMsg, TransitionResultMsg:
		if d.picker != nil {
			p, cmd := d.picker.Update(msg)
			d.picker = &p
			return d, cmd
		}
	case ClosePickerMsg:
		d.picker = nil
	case tea.KeyMsg:
		return d.handleKey(msg)
	}
	return d, nil
}

func (d Dashboard) handleKey(k tea.KeyMsg) (Dashboard, tea.Cmd) {
	if k.String() == "ctrl+c" {
		return d, tea.Quit
	}
	if d.picker != nil {
		p, cmd := d.picker.Update(k)
		d.picker = &p
		return d, cmd
	}
	if d.detail != nil {
		switch k.String() {
		case "esc", "q":
			d.detail = nil
		case "t":
			return d.openPicker(d.detail)
		case "o":
			return d, d.open(d.detail)
		}
		return d, nil
	}
	if d.showHelp {
		switch k.String() {
		case "?", "esc":
			d.showHelp = false
		case "q":
			return d, tea.Quit
		}
		return d, nil
	}
	switch k.String() {
	case "?":
		d.showHelp = true
	case "q":
		return d, tea.Quit
	case "v":
		d.mode = 1 - d.mode
	case "r":
		return d, func() tea.Msg { return RefreshMsg{} }
	case "m":
		groups := model.ByEpic(d.snap.Cards)
		return d, func() tea.Msg { return OpenManageMsg{Groups: groups} }
	case "d":
		d.showDone = !d.showDone
		d.relayout()
	case "o":
		return d, d.open(d.Selected())
	case "t":
		return d.openPicker(d.Selected())
	case "enter":
		if r := d.listRow(); d.listShown() && r != nil && r.Group != nil {
			d.collapsed = maps.Clone(d.collapsed) // earlier copies keep their state
			d.collapsed[r.Group.Key] = !d.collapsed[r.Group.Key]
		} else if c := d.Selected(); c != nil {
			cp := *c
			d.detail = &cp
		}
	case "up", "k":
		d.move(-1)
	case "down", "j":
		d.move(1)
	case "left", "h":
		if !d.listShown() {
			d.moveCol(-1)
		}
	case "right", "l":
		if !d.listShown() {
			d.moveCol(1)
		}
	}
	return d, nil
}

// refreshDetail swaps the detail's card for its copy in the new snapshot. A
// card that left the board keeps its last detail.
func (d *Dashboard) refreshDetail() {
	if d.detail == nil {
		return
	}
	for _, c := range d.snap.Cards {
		if c.Key == d.detail.Key {
			d.detail = &c
			return
		}
	}
}

// open is a command that opens c in the browser, or nil when c has no URL.
func (d Dashboard) open(c *model.Card) tea.Cmd {
	if c == nil || c.URL == "" {
		return nil
	}
	open, u := d.openURL, c.URL
	return func() tea.Msg { _ = open(u); return nil }
}

// openPicker shows the transition picker for c and asks main for its
// transitions.
func (d Dashboard) openPicker(c *model.Card) (Dashboard, tea.Cmd) {
	if c == nil {
		return d, nil
	}
	p, k := NewPicker(c.Key), c.Key
	d.picker = &p
	return d, func() tea.Msg { return LoadTransitionsMsg{Key: k} }
}

// relayout rebuilds both views from the snapshot, keeping each view's
// selection on the same card (or list group) when it is still there.
func (d *Dashboard) relayout() {
	kanbanKey := ""
	if c := d.kanbanSelected(); c != nil {
		kanbanKey = c.Key
	}
	listRow := d.listRow()
	cards := d.snap.Cards
	if !d.showDone {
		cards = nil
		for _, c := range d.snap.Cards {
			if c.Lane != model.LaneDone {
				cards = append(cards, c)
			}
		}
	}
	d.board = model.Layout(d.snap.Columns, cards, d.showDone)
	d.groups = model.ByEpic(cards)
	d.findKanban(kanbanKey)
	d.findListRow(listRow)
	d.clamp()
}

// findKanban puts the kanban cursor on the card with key, if it is shown.
func (d *Dashboard) findKanban(key string) {
	for _, l := range visibleLanes(d.board) {
		for ci := range d.board.Columns {
			for ri, c := range d.cell(l, ci) {
				if key != "" && c.Key == key {
					d.cur = Cursor{Lane: l, Col: ci, Row: ri}
					return
				}
			}
		}
	}
}

// findListRow selects the list row for the same card or group as r.
func (d *Dashboard) findListRow(r *ListRow) {
	if r == nil {
		return
	}
	for i, x := range ListRows(d.groups, d.collapsed) {
		if (r.Card != nil && x.Card != nil && x.Card.Key == r.Card.Key) ||
			(r.Group != nil && x.Group != nil && x.Group.Key == r.Group.Key) {
			d.listSel = i
			return
		}
	}
}

// listRow is the selected list row, or nil.
func (d Dashboard) listRow() *ListRow {
	if rows := ListRows(d.groups, d.collapsed); d.listSel < len(rows) {
		return &rows[d.listSel]
	}
	return nil
}

// cell is the kanban cell at lane l, column index col.
func (d Dashboard) cell(l model.Lane, col int) []model.Card {
	if col < 0 || col >= len(d.board.Columns) {
		return nil
	}
	return d.board.Cell(l, d.board.Columns[col])
}

// clamp keeps the kanban cursor on a card: the last row of a cell that
// shrank, or else the board's first card.
func (d *Dashboard) clamp() {
	d.listSel = min(d.listSel, max(len(ListRows(d.groups, d.collapsed))-1, 0))
	if n := len(d.cell(d.cur.Lane, d.cur.Col)); n > 0 {
		d.cur.Row = min(max(d.cur.Row, 0), n-1)
		return
	}
	for _, l := range visibleLanes(d.board) {
		for ci := range d.board.Columns {
			if len(d.cell(l, ci)) > 0 {
				d.cur = Cursor{Lane: l, Col: ci}
				return
			}
		}
	}
	d.cur = Cursor{}
}

// moveCol moves to the nearest column in direction dx that has a card in
// the current lane, keeping the row where the new cell is tall enough.
func (d *Dashboard) moveCol(dx int) {
	for ci := d.cur.Col + dx; ci >= 0 && ci < len(d.board.Columns); ci += dx {
		if n := len(d.cell(d.cur.Lane, ci)); n > 0 {
			d.cur.Col, d.cur.Row = ci, min(d.cur.Row, n-1)
			return
		}
	}
}

// minColWidth is the narrowest kanban column RenderKanban draws.
const minColWidth = 12

// tooNarrow reports whether the terminal cannot fit the kanban's columns.
func (d Dashboard) tooNarrow() bool {
	return d.width < minColWidth*len(d.board.Columns)
}

// listShown reports whether the list is on screen: chosen, or standing in
// for a kanban that does not fit.
func (d Dashboard) listShown() bool { return d.mode == ViewList || d.tooNarrow() }

func (d *Dashboard) move(dy int) {
	if d.listShown() {
		n := len(ListRows(d.groups, d.collapsed))
		d.listSel = min(max(d.listSel+dy, 0), max(n-1, 0))
		return
	}
	d.moveRow(dy)
}

// moveRow moves one card down (dy 1) or up (-1) the current column. Past
// the end of a cell it hops to the nearest lane with a card in this column.
func (d *Dashboard) moveRow(dy int) {
	if r := d.cur.Row + dy; r >= 0 && r < len(d.cell(d.cur.Lane, d.cur.Col)) {
		d.cur.Row = r
		return
	}
	lanes := visibleLanes(d.board)
	for li := slices.Index(lanes, d.cur.Lane) + dy; li >= 0 && li < len(lanes); li += dy {
		if n := len(d.cell(lanes[li], d.cur.Col)); n > 0 {
			d.cur.Lane, d.cur.Row = lanes[li], 0
			if dy < 0 {
				d.cur.Row = n - 1
			}
			return
		}
	}
}

// Selected is the card under the cursor, or nil when there is none.
func (d Dashboard) Selected() *model.Card {
	if d.listShown() {
		if r := d.listRow(); r != nil {
			return r.Card
		}
		return nil
	}
	return d.kanbanSelected()
}

func (d Dashboard) kanbanSelected() *model.Card {
	if cell := d.cell(d.cur.Lane, d.cur.Col); d.cur.Row < len(cell) {
		return &cell[d.cur.Row]
	}
	return nil
}

// View renders the dashboard.
func (d Dashboard) View() string {
	if d.picker != nil {
		return d.picker.View()
	}
	if d.detail != nil {
		return RenderDetail(*d.detail, d.width)
	}
	if d.showHelp {
		return RenderHelp()
	}
	var body string
	if d.listShown() {
		body = RenderList(d.groups, d.collapsed, d.listSel, d.width)
		if d.mode == ViewKanban {
			hint := fmt.Sprintf("kanban needs %d cols - showing list", minColWidth*len(d.board.Columns))
			body = dimStyle.Render(truncate(hint, d.width)) + "\n" + body
		}
	} else {
		body = RenderKanban(d.board, d.cur, d.width)
	}
	return strings.TrimSuffix(body, "\n") + "\n" + d.statusLine()
}

const keyHints = "←→↑↓ move  enter detail  t transition  o open  v view  d done  r refresh  m manage  q quit  ? help"

func (d Dashboard) statusLine() string {
	synced := "never synced"
	if !d.snap.At.IsZero() {
		synced = "updated " + d.snap.At.Format("15:04:05")
	}
	parts := []string{synced, keyHints}
	switch {
	case d.snap.AuthFailed:
		parts = append([]string{"token rejected - check token_command"}, parts...)
	case d.snap.Err != nil:
		parts = append([]string{model.Stale.Emoji() + " stale: " + d.snap.Err.Error()}, parts...)
	}
	return truncate(dimStyle.Render(strings.Join(parts, "  ·  ")), d.width)
}

// OpenBrowser opens u in the default browser without waiting for it.
func OpenBrowser(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}
