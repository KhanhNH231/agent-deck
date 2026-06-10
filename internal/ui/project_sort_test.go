package ui

import (
	"testing"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootGroup(path string) session.Item {
	return session.Item{Type: session.ItemTypeGroup, Level: 0, Path: path,
		Group: &session.Group{Path: path, Name: path}}
}

func sess(status session.Status) session.Item {
	return session.Item{Type: session.ItemTypeSession, Level: 1,
		Session: &session.Instance{Status: status}}
}

// rootOrder extracts the Level-0 group paths, in order, from a flat item list.
func rootOrder(items []session.Item) []string {
	var out []string
	for _, it := range items {
		if it.Type == session.ItemTypeGroup && it.Level == 0 {
			out = append(out, it.Path)
		}
	}
	return out
}

func TestProjectSortModeToggleAndLabel(t *testing.T) {
	assert.Equal(t, projectSortStatus, projectSortManual.next())
	assert.Equal(t, projectSortManual, projectSortStatus.next())
	assert.Equal(t, "Projects: manual order", projectSortManual.label())
	assert.Equal(t, "Projects: by status", projectSortStatus.label())
}

func TestReorderRootGroupBlocksByStatus_MostActionableFirst(t *testing.T) {
	// alpha: only running; beta: has a waiting session; gamma: idle.
	// Expected order by actionable priority: beta (waiting=1) < alpha (running=2)
	// < gamma (idle=3).
	items := []session.Item{
		rootGroup("alpha"), sess(session.StatusRunning),
		rootGroup("beta"), sess(session.StatusIdle), sess(session.StatusWaiting),
		rootGroup("gamma"), sess(session.StatusIdle),
	}

	got := reorderRootGroupBlocksByStatus(items)
	assert.Equal(t, []string{"beta", "alpha", "gamma"}, rootOrder(got))

	// Every block keeps its own sessions contiguous: beta still owns 2 sessions.
	// Find beta header, assert next two items are its sessions.
	for i, it := range got {
		if it.Type == session.ItemTypeGroup && it.Path == "beta" {
			require.Less(t, i+2, len(got))
			assert.Equal(t, session.ItemTypeSession, got[i+1].Type)
			assert.Equal(t, session.ItemTypeSession, got[i+2].Type)
		}
	}
}

func TestReorderRootGroupBlocksByStatus_ConductorPinnedAndStableTiebreak(t *testing.T) {
	// conductor pinned top even though it only has an idle session; the two
	// equal-priority idle projects keep their incoming (manual) order.
	items := []session.Item{
		rootGroup("alpha"), sess(session.StatusIdle),
		rootGroup("conductor"), sess(session.StatusIdle),
		rootGroup("beta"), sess(session.StatusError),
	}

	got := reorderRootGroupBlocksByStatus(items)
	// conductor first; then beta (error=0); then alpha (idle).
	assert.Equal(t, []string{"conductor", "beta", "alpha"}, rootOrder(got))
}

func TestReorderRootGroupBlocksByStatus_PrefixPreserved(t *testing.T) {
	// A placeholder item before any root group must stay first, untouched.
	placeholder := session.Item{Type: session.ItemTypeSession, Level: 1,
		Session: &session.Instance{Status: session.StatusWaiting}, CreatingID: "x"}
	items := []session.Item{
		placeholder,
		rootGroup("alpha"), sess(session.StatusIdle),
		rootGroup("beta"), sess(session.StatusError),
	}

	got := reorderRootGroupBlocksByStatus(items)
	assert.Equal(t, "x", got[0].CreatingID, "prefix placeholder stays first")
	assert.Equal(t, []string{"beta", "alpha"}, rootOrder(got))
}
