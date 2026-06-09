package ui

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func ids(instances []*session.Instance) []string {
	out := make([]string, 0, len(instances))
	for _, inst := range instances {
		out = append(out, inst.ID)
	}
	return out
}

func TestSortSessionsByRecency_RealTimestampsDESC(t *testing.T) {
	now := time.Now()
	a := &session.Instance{ID: "a", LastAccessedAt: now.Add(-3 * time.Hour)}
	b := &session.Instance{ID: "b", LastAccessedAt: now.Add(-1 * time.Hour)}
	c := &session.Instance{ID: "c", LastAccessedAt: now.Add(-2 * time.Hour)}

	got := ids(sortSessionsByRecency([]*session.Instance{a, b, c}))
	want := []string{"b", "c", "a"} // most-recently-accessed first

	if !equalStrings(got, want) {
		t.Fatalf("recency order = %v, want %v", got, want)
	}
}

func TestSortSessionsByRecency_ZeroTimestampSinksToBottom(t *testing.T) {
	now := time.Now()
	accessed := &session.Instance{ID: "accessed", LastAccessedAt: now.Add(-5 * time.Hour)}
	// "never" has no LastAccessedAt but a very recent CreatedAt; it must still
	// sink below any attached session.
	never := &session.Instance{ID: "never", CreatedAt: now}

	got := ids(sortSessionsByRecency([]*session.Instance{never, accessed}))
	want := []string{"accessed", "never"}

	if !equalStrings(got, want) {
		t.Fatalf("zero-timestamp order = %v, want %v (never-attached must sink)", got, want)
	}
}

func TestSortSessionsByRecency_ZeroTimestampTierUsesBadgeTime(t *testing.T) {
	now := time.Now()
	// Both never attached; the one with the newer badge time (CreatedAt here)
	// must come first within the never-attached tier.
	older := &session.Instance{ID: "older", CreatedAt: now.Add(-4 * time.Hour)}
	newer := &session.Instance{ID: "newer", CreatedAt: now.Add(-1 * time.Hour)}

	got := ids(sortSessionsByRecency([]*session.Instance{older, newer}))
	want := []string{"newer", "older"}

	if !equalStrings(got, want) {
		t.Fatalf("never-attached badge order = %v, want %v", got, want)
	}
}

func TestSortSessionsByRecency_StableTieBreakByOrderThenTitle(t *testing.T) {
	ts := time.Now().Add(-1 * time.Hour)
	// Identical LastAccessedAt → tie-break by Order, then Title.
	o2 := &session.Instance{ID: "o2", LastAccessedAt: ts, Order: 2, Title: "zeta"}
	o1b := &session.Instance{ID: "o1b", LastAccessedAt: ts, Order: 1, Title: "beta"}
	o1a := &session.Instance{ID: "o1a", LastAccessedAt: ts, Order: 1, Title: "alpha"}

	got := ids(sortSessionsByRecency([]*session.Instance{o2, o1b, o1a}))
	want := []string{"o1a", "o1b", "o2"} // Order 1 (alpha, beta) before Order 2

	if !equalStrings(got, want) {
		t.Fatalf("tie-break order = %v, want %v", got, want)
	}
}

func TestSortSessionsByRecency_EmptyAndSingle(t *testing.T) {
	if got := sortSessionsByRecency(nil); len(got) != 0 {
		t.Fatalf("empty input = %v, want empty", ids(got))
	}
	one := &session.Instance{ID: "only", LastAccessedAt: time.Now()}
	got := ids(sortSessionsByRecency([]*session.Instance{one}))
	if !equalStrings(got, []string{"only"}) {
		t.Fatalf("single input = %v, want [only]", got)
	}
}

func TestSortSessionsByRecency_DoesNotMutateInput(t *testing.T) {
	now := time.Now()
	a := &session.Instance{ID: "a", LastAccessedAt: now.Add(-3 * time.Hour)}
	b := &session.Instance{ID: "b", LastAccessedAt: now.Add(-1 * time.Hour)}
	in := []*session.Instance{a, b}
	_ = sortSessionsByRecency(in)
	if in[0].ID != "a" || in[1].ID != "b" {
		t.Fatalf("input slice was mutated: %v", ids(in))
	}
}

func TestFilterSessions_EmptyQueryReturnsAll(t *testing.T) {
	a := &session.Instance{ID: "a", Title: "alpha"}
	b := &session.Instance{ID: "b", Title: "beta"}
	got := ids(filterSessions([]*session.Instance{a, b}, ""))
	if !equalStrings(got, []string{"a", "b"}) {
		t.Fatalf("empty query = %v, want all", got)
	}
	// Whitespace-only query also returns all.
	got = ids(filterSessions([]*session.Instance{a, b}, "   "))
	if !equalStrings(got, []string{"a", "b"}) {
		t.Fatalf("whitespace query = %v, want all", got)
	}
}

func TestFilterSessions_CaseInsensitiveTitleMatch(t *testing.T) {
	a := &session.Instance{ID: "a", Title: "Deploy Pipeline"}
	b := &session.Instance{ID: "b", Title: "Frontend"}
	got := ids(filterSessions([]*session.Instance{a, b}, "DEPLOY"))
	if !equalStrings(got, []string{"a"}) {
		t.Fatalf("case-insensitive title match = %v, want [a]", got)
	}
}

func TestFilterSessions_MatchesProjectAndGroupPath(t *testing.T) {
	a := &session.Instance{ID: "a", Title: "x", ProjectPath: "/work/agent-deck"}
	b := &session.Instance{ID: "b", Title: "y", GroupPath: "projects/devops"}
	c := &session.Instance{ID: "c", Title: "z", ProjectPath: "/tmp/other"}

	// Matches via ProjectPath.
	if got := ids(filterSessions([]*session.Instance{a, b, c}, "agent-deck")); !equalStrings(got, []string{"a"}) {
		t.Fatalf("project path match = %v, want [a]", got)
	}
	// Matches via GroupPath.
	if got := ids(filterSessions([]*session.Instance{a, b, c}, "devops")); !equalStrings(got, []string{"b"}) {
		t.Fatalf("group path match = %v, want [b]", got)
	}
}

func TestFilterSessions_NoMatchReturnsEmpty(t *testing.T) {
	a := &session.Instance{ID: "a", Title: "alpha", ProjectPath: "/p", GroupPath: "g"}
	got := filterSessions([]*session.Instance{a}, "nonexistent")
	if len(got) != 0 {
		t.Fatalf("no-match = %v, want empty", ids(got))
	}
}

func TestRecentSwitcher_ShowOrdersByRecencyAndSelectsTop(t *testing.T) {
	now := time.Now()
	a := &session.Instance{ID: "a", Title: "A", LastAccessedAt: now.Add(-3 * time.Hour)}
	b := &session.Instance{ID: "b", Title: "B", LastAccessedAt: now.Add(-1 * time.Hour)}

	rs := NewRecentSwitcher()
	rs.Show([]*session.Instance{a, b})

	if !rs.IsVisible() {
		t.Fatal("expected switcher visible after Show")
	}
	if sel := rs.Selected(); sel == nil || sel.ID != "b" {
		t.Fatalf("default selection = %v, want most-recent b", sel)
	}
}

func TestRecentSwitcher_DownThenEnterSelectsSecondAndCloses(t *testing.T) {
	now := time.Now()
	a := &session.Instance{ID: "a", Title: "A", LastAccessedAt: now.Add(-3 * time.Hour)}
	b := &session.Instance{ID: "b", Title: "B", LastAccessedAt: now.Add(-1 * time.Hour)}

	rs := NewRecentSwitcher()
	rs.Show([]*session.Instance{a, b})
	rs, _ = rs.Update(keyMsg("down")) // move to second row ("a")

	sel := rs.Selected()
	if sel == nil || sel.ID != "a" {
		t.Fatalf("after Down selection = %v, want a", sel)
	}

	rs, _ = rs.Update(keyMsg("enter"))
	if rs.IsVisible() {
		t.Fatal("expected switcher hidden after Enter")
	}
}

func TestRecentSwitcher_EscClosesWithNoChange(t *testing.T) {
	rs := NewRecentSwitcher()
	rs.Show([]*session.Instance{{ID: "a", Title: "A"}})
	rs, _ = rs.Update(keyMsg("esc"))
	if rs.IsVisible() {
		t.Fatal("expected switcher hidden after Esc")
	}
}

func TestRecentSwitcher_TypingFiltersList(t *testing.T) {
	a := &session.Instance{ID: "a", Title: "deploy"}
	b := &session.Instance{ID: "b", Title: "frontend"}

	rs := NewRecentSwitcher()
	rs.Show([]*session.Instance{a, b})
	for _, ch := range "front" {
		rs, _ = rs.Update(keyMsg(string(ch)))
	}

	sel := rs.Selected()
	if sel == nil || sel.ID != "b" {
		t.Fatalf("after typing 'front' selection = %v, want b", sel)
	}
}

func TestRecentSwitcher_FilterToEmptyKeepsOverlayOpenWithNilSelection(t *testing.T) {
	rs := NewRecentSwitcher()
	rs.Show([]*session.Instance{{ID: "a", Title: "deploy"}})
	for _, ch := range "zzz" {
		rs, _ = rs.Update(keyMsg(string(ch)))
	}
	if !rs.IsVisible() {
		t.Fatal("overlay should stay open when filter matches nothing")
	}
	if sel := rs.Selected(); sel != nil {
		t.Fatalf("no-match selection = %v, want nil", sel)
	}
	// Enter on empty list must not panic and should close.
	rs, _ = rs.Update(keyMsg("enter"))
	if rs.IsVisible() {
		t.Fatal("Enter on empty list should close overlay")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
