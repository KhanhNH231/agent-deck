package ui

import (
	"sort"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// Project sort mode: how root groups (projects) are ordered in the left list.
//
//	manual  — user-customized Order (the default; today's behavior, untouched)
//	status  — most-actionable project first (error > needs-input > running >
//	          idle > stopped), manual Order kept as the stable tiebreak
//
// The toggle is UI-only: it reorders flatItems at render-build time and never
// mutates the persisted group Order or the web view.
type projectSortMode int

const (
	projectSortManual projectSortMode = iota
	projectSortStatus
)

// next returns the mode reached by cycling the toggle once.
func (m projectSortMode) next() projectSortMode {
	if m == projectSortManual {
		return projectSortStatus
	}
	return projectSortManual
}

// label is the human-readable status-banner text for a mode.
func (m projectSortMode) label() string {
	if m == projectSortStatus {
		return "Projects: by status"
	}
	return "Projects: manual order"
}

// blockPriority is the aggregate actionable priority of one root-group block:
// the most urgent (lowest) priority among its session items. A block with no
// sessions sorts after every block that has one.
const noSessionBlockPriority = 1 << 30

func blockPriority(block []session.Item) int {
	best := noSessionBlockPriority
	for _, it := range block {
		if it.Type == session.ItemTypeSession && it.Session != nil {
			if p := session.ActionablePriority(it.Session.GetStatusThreadSafe()); p < best {
				best = p
			}
		}
	}
	return best
}

// reorderRootGroupBlocksByStatus reorders the root-group blocks of a flattened
// item list so the most-actionable project comes first. A "block" is a Level-0
// group header plus everything up to (but not including) the next Level-0 group
// — i.e. its sessions, windows, and subgroups, kept contiguous.
//
// Items appearing before the first root group (e.g. placeholders) are preserved
// as an untouched prefix. The "conductor" group, if present, stays pinned at the
// top. The sort is stable, so blocks of equal priority keep their incoming
// (manual) order. RootGroupNum is NOT assigned here — the caller renumbers after
// reordering.
func reorderRootGroupBlocksByStatus(items []session.Item) []session.Item {
	// Find the first root-group header; everything before it is a fixed prefix.
	firstRoot := -1
	for i, it := range items {
		if it.Type == session.ItemTypeGroup && it.Level == 0 {
			firstRoot = i
			break
		}
	}
	if firstRoot == -1 {
		return items // no root groups to reorder
	}

	prefix := items[:firstRoot]

	// Partition the remainder into contiguous blocks, each starting at a
	// Level-0 group header.
	type rootBlock struct {
		items     []session.Item
		conductor bool
		priority  int
		order     int // incoming index, for a stable tiebreak
	}
	var blocks []rootBlock
	cur := -1
	for i := firstRoot; i < len(items); i++ {
		it := items[i]
		if it.Type == session.ItemTypeGroup && it.Level == 0 {
			blocks = append(blocks, rootBlock{
				order:     len(blocks),
				conductor: it.Path == "conductor",
			})
			cur = len(blocks) - 1
		}
		if cur >= 0 {
			blocks[cur].items = append(blocks[cur].items, it)
		}
	}
	for i := range blocks {
		blocks[i].priority = blockPriority(blocks[i].items)
	}

	sort.SliceStable(blocks, func(i, j int) bool {
		// Conductor pinned to the very top.
		if blocks[i].conductor != blocks[j].conductor {
			return blocks[i].conductor
		}
		if blocks[i].priority != blocks[j].priority {
			return blocks[i].priority < blocks[j].priority
		}
		return blocks[i].order < blocks[j].order
	})

	out := make([]session.Item, 0, len(items))
	out = append(out, prefix...)
	for _, b := range blocks {
		out = append(out, b.items...)
	}
	return out
}
