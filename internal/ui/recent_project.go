package ui

import (
	"strings"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Ctrl+E "back to most recent project" (alt-tab style).
//
// A "project" is the root group a session lives under — the Level-0 folder the
// TUI shows at the top level (the same unit the 1-9 quick-jump keys target).
// The jump is stateless: it picks the most-recently-worked session whose
// project differs from the cursor's. Because recency (LastAccessedAt) only
// advances on attach, not on cursor movement, repeated presses bounce between
// the last two projects — true alt-tab behavior with no history to maintain.

// topLevelGroup returns the root-group segment of a group path.
// "feature/sub" -> "feature"; "feature" -> "feature"; "" -> "".
func topLevelGroup(groupPath string) string {
	if groupPath == "" {
		return ""
	}
	if i := strings.Index(groupPath, "/"); i >= 0 {
		return groupPath[:i]
	}
	return groupPath
}

// projectKeyOf returns a stable identifier for the project a session belongs to:
// its root group when grouped, otherwise its project path. The "g:" / "p:"
// prefixes keep a group named "foo" distinct from a project path "foo".
func projectKeyOf(inst *session.Instance) string {
	if inst == nil {
		return ""
	}
	if tl := topLevelGroup(inst.GroupPath); tl != "" {
		return "g:" + tl
	}
	return "p:" + inst.ProjectPath
}

// cursorProjectKey returns the project key the cursor is currently inside, or ""
// when it cannot be determined.
func (h *Home) cursorProjectKey() string {
	if h.cursor < 0 || h.cursor >= len(h.flatItems) {
		return ""
	}
	it := h.flatItems[h.cursor]
	switch it.Type {
	case session.ItemTypeSession:
		return projectKeyOf(it.Session)
	case session.ItemTypeGroup:
		if tl := topLevelGroup(it.Path); tl != "" {
			return "g:" + tl
		}
	case session.ItemTypeWindow:
		if tl := topLevelGroup(h.currentGroupPath()); tl != "" {
			return "g:" + tl
		}
	}
	return ""
}

// recentProjectTarget returns the most-recently-worked session whose project
// differs from the cursor's current project, or nil when no other project
// exists. Recency uses the same ordering as the recent-switcher overlay.
func (h *Home) recentProjectTarget() *session.Instance {
	cur := h.cursorProjectKey()

	h.instancesMu.RLock()
	items := make([]*session.Instance, len(h.instances))
	copy(items, h.instances)
	h.instancesMu.RUnlock()

	for _, inst := range sortSessionsByRecency(items) {
		if inst == nil {
			continue
		}
		if projectKeyOf(inst) != cur {
			return inst
		}
	}
	return nil
}
