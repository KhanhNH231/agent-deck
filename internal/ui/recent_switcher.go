package ui

import (
	"sort"
	"strings"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// maxRecentSwitcherRows caps how many rows the overlay draws so a large session
// count cannot blow past the terminal height.
const maxRecentSwitcherRows = 12

// recencyKey is the comparison key for one session in the recent switcher.
// hasAccessed splits the list into two tiers: sessions the user has actually
// attached to (a real LastAccessedAt) always sort ahead of never-attached
// ones, regardless of badge time. Within a tier, the more-recent badgeTime
// wins, then Order then Title give a stable, deterministic tie-break.
type recencyKey struct {
	hasAccessed bool
	badgeTime   time.Time
	order       int
	title       string
}

// recencyKeyFor computes the sort key for one instance. LastAccessedAt (set by
// MarkAccessed() on attach) is the primary signal; when it is zero the session
// falls back to the same badge time the row badge shows (pickBadgeTime over
// CreatedAt / LastStartedAt / hook UpdatedAt / confirmed activity) so ordering
// matches the user-visible relative-time badge.
func recencyKeyFor(inst *session.Instance) recencyKey {
	if inst == nil {
		return recencyKey{}
	}

	hasAccessed := !inst.LastAccessedAt.IsZero()
	badge := inst.LastAccessedAt
	if !hasAccessed {
		var hookEvent *session.HookStatus
		confirmedActivity, confirmedObserved := inst.LastObservedActivity()
		badge = pickBadgeTime(inst.CreatedAt, inst.LastStartedAt, hookEvent, confirmedActivity, confirmedObserved)
	}

	return recencyKey{
		hasAccessed: hasAccessed,
		badgeTime:   badge,
		order:       inst.Order,
		title:       inst.Title,
	}
}

// lessRecency reports whether instance a should sort before instance b in the
// recent switcher (most-recently-worked first). It is a strict weak ordering
// suitable for sort.SliceStable.
func lessRecency(a, b *session.Instance) bool {
	ka := recencyKeyFor(a)
	kb := recencyKeyFor(b)

	// Tier 1: attached sessions before never-attached ones.
	if ka.hasAccessed != kb.hasAccessed {
		return ka.hasAccessed
	}

	// Within a tier: more recent badge time first.
	if !ka.badgeTime.Equal(kb.badgeTime) {
		return ka.badgeTime.After(kb.badgeTime)
	}

	// Stable tie-break: lower Order, then title (case-insensitive).
	if ka.order != kb.order {
		return ka.order < kb.order
	}
	return strings.ToLower(ka.title) < strings.ToLower(kb.title)
}

// sortSessionsByRecency returns a new slice ordered most-recently-worked-first.
// The input slice is not mutated. nil instances are dropped.
func sortSessionsByRecency(instances []*session.Instance) []*session.Instance {
	out := make([]*session.Instance, 0, len(instances))
	for _, inst := range instances {
		if inst != nil {
			out = append(out, inst)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return lessRecency(out[i], out[j])
	})
	return out
}

// filterSessions returns the instances whose title or project/group path
// contains query (case-insensitive substring). An empty/whitespace query
// returns every instance. The input order is preserved.
func filterSessions(instances []*session.Instance, query string) []*session.Instance {
	trimmed := strings.ToLower(strings.TrimSpace(query))
	if trimmed == "" {
		out := make([]*session.Instance, 0, len(instances))
		for _, inst := range instances {
			if inst != nil {
				out = append(out, inst)
			}
		}
		return out
	}

	out := make([]*session.Instance, 0, len(instances))
	for _, inst := range instances {
		if inst == nil {
			continue
		}
		if strings.Contains(strings.ToLower(inst.Title), trimmed) ||
			strings.Contains(strings.ToLower(inst.ProjectPath), trimmed) ||
			strings.Contains(strings.ToLower(inst.GroupPath), trimmed) {
			out = append(out, inst)
		}
	}
	return out
}

// RecentSwitcher is the quick-switch overlay: a recency-ordered, fuzzy-filtered
// list of every active session across all projects. Enter attaches the selected
// session (via the parent's existing attachSession path); Esc closes with no
// change. It mirrors the Search overlay idiom (Show/Hide/IsVisible/Update/View)
// so it slots into the same modal-priority chain.
type RecentSwitcher struct {
	input    textinput.Model
	all      []*session.Instance // recency-ordered, set at Show()
	filtered []*session.Instance // all ∩ current query
	cursor   int
	width    int
	height   int
	visible  bool
}

// NewRecentSwitcher builds an empty, hidden overlay.
func NewRecentSwitcher() *RecentSwitcher {
	ti := textinput.New()
	ti.Placeholder = "Jump to session..."
	ti.CharLimit = 100
	ti.Width = 50
	return &RecentSwitcher{
		input:    ti,
		filtered: []*session.Instance{},
	}
}

// SetSize records the terminal dimensions for centering.
func (r *RecentSwitcher) SetSize(width, height int) {
	r.width = width
	r.height = height
}

// Show opens the overlay over the given session set, ordering it
// most-recently-worked-first and resetting the query and cursor.
func (r *RecentSwitcher) Show(instances []*session.Instance) {
	r.all = sortSessionsByRecency(instances)
	r.input.SetValue("")
	r.input.Focus()
	r.cursor = 0
	r.visible = true
	r.refilter()
}

// Hide closes the overlay.
func (r *RecentSwitcher) Hide() {
	r.visible = false
	r.input.Blur()
}

// IsVisible reports whether the overlay is open. Nil-safe so manual Home
// builders that omit the field do not panic in the modal-priority chain.
func (r *RecentSwitcher) IsVisible() bool {
	return r != nil && r.visible
}

// Selected returns the highlighted instance, or nil when the list is empty.
func (r *RecentSwitcher) Selected() *session.Instance {
	if len(r.filtered) == 0 {
		return nil
	}
	if r.cursor >= len(r.filtered) {
		r.cursor = len(r.filtered) - 1
	}
	return r.filtered[r.cursor]
}

// refilter recomputes the visible list from the current query and clamps the
// cursor. Recency order is preserved because filterSessions keeps input order.
func (r *RecentSwitcher) refilter() {
	r.filtered = filterSessions(r.all, r.input.Value())
	if r.cursor >= len(r.filtered) {
		r.cursor = len(r.filtered) - 1
	}
	if r.cursor < 0 {
		r.cursor = 0
	}
}

// Update handles key input. It never attaches itself: on Enter it hides and the
// parent reads Selected() and routes through attachSession. Esc closes with no
// change. Returns the updated overlay and any textinput command.
func (r *RecentSwitcher) Update(msg tea.Msg) (*RecentSwitcher, tea.Cmd) {
	if !r.visible {
		return r, nil
	}

	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return r, nil
	}

	switch key.String() {
	case "esc":
		r.Hide()
		return r, nil
	case "enter":
		// Parent inspects Selected() before/after; just close here.
		r.Hide()
		return r, nil
	case "up", "ctrl+p", "ctrl+k":
		if r.cursor > 0 {
			r.cursor--
		}
		return r, nil
	case "down", "ctrl+n", "ctrl+j":
		if r.cursor < len(r.filtered)-1 {
			r.cursor++
		}
		return r, nil
	default:
		var cmd tea.Cmd
		r.input, cmd = r.input.Update(msg)
		r.refilter()
		return r, cmd
	}
}

// View renders the centered overlay. Each row shows the recency badge, status,
// title, and project/group path; the selected row is highlighted.
func (r *RecentSwitcher) View() string {
	if !r.visible {
		return ""
	}

	header := lipgloss.NewStyle().
		Foreground(ColorAccent).
		Bold(true).
		Render("⏎ Recent Sessions (all projects)")

	searchBox := searchBoxStyle.Render(r.input.View())

	var body string
	if len(r.filtered) == 0 {
		emptyMsg := "No sessions"
		if r.input.Value() != "" {
			emptyMsg = "No matching sessions"
		}
		body = lipgloss.NewStyle().
			Foreground(ColorComment).
			Italic(true).
			Render("  " + emptyMsg)
	} else {
		body = r.renderRows()
	}

	keysHint := lipgloss.NewStyle().
		Foreground(ColorComment).
		Render("  [Enter] Jump  [↑↓ / Ctrl+P/N] Navigate  [Esc] Cancel")

	content := header + "\n\n" + searchBox + "\n\n" + body + "\n\n" + keysHint

	overlayWidth := 70
	if r.width > 0 && r.width < overlayWidth+10 {
		overlayWidth = r.width - 10
		if overlayWidth < 30 {
			overlayWidth = 30
		}
	}
	overlay := overlayStyle.Width(overlayWidth).Render(content)
	return centerInScreen(overlay, r.width, r.height)
}

// renderRows draws up to maxRecentSwitcherRows rows, keeping the cursor visible
// via a simple scrolling window.
func (r *RecentSwitcher) renderRows() string {
	start := 0
	if r.cursor >= maxRecentSwitcherRows {
		start = r.cursor - maxRecentSwitcherRows + 1
	}
	end := start + maxRecentSwitcherRows
	if end > len(r.filtered) {
		end = len(r.filtered)
	}

	var rows strings.Builder
	for i := start; i < end; i++ {
		inst := r.filtered[i]
		row := r.formatRow(inst)
		if i == r.cursor {
			rows.WriteString(selectedResultStyle.Render("› " + row))
		} else {
			rows.WriteString(resultItemStyle.Render("  " + row))
		}
		if i < end-1 {
			rows.WriteString("\n")
		}
	}
	return rows.String()
}

// formatRow builds one row's text: "title  [status] · group/project · 2h ago".
// It reuses formatRelativeTime and the same recency time used for ordering so
// the badge matches the sort.
func (r *RecentSwitcher) formatRow(inst *session.Instance) string {
	badge := recencyKeyFor(inst).badgeTime
	rel := formatRelativeTime(badge)
	status := string(inst.GetStatusThreadSafe())

	location := inst.GroupPath
	if location == "" {
		location = inst.ProjectPath
	}

	parts := []string{inst.Title}
	if status != "" {
		parts = append(parts, "["+status+"]")
	}
	if location != "" {
		parts = append(parts, "· "+location)
	}
	parts = append(parts, "· "+rel)
	return strings.Join(parts, "  ")
}
