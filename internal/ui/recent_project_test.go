package ui

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestTopLevelGroup(t *testing.T) {
	assert.Equal(t, "", topLevelGroup(""))
	assert.Equal(t, "feature", topLevelGroup("feature"))
	assert.Equal(t, "feature", topLevelGroup("feature/sub"))
	assert.Equal(t, "feature", topLevelGroup("feature/sub/deep"))
}

func TestProjectKeyOf(t *testing.T) {
	assert.Equal(t, "", projectKeyOf(nil))

	grouped := &session.Instance{GroupPath: "feat-x/repo-a", ProjectPath: "/home/u/repo-a"}
	assert.Equal(t, "g:feat-x", projectKeyOf(grouped))

	ungrouped := &session.Instance{GroupPath: "", ProjectPath: "/home/u/repo-a"}
	assert.Equal(t, "p:/home/u/repo-a", projectKeyOf(ungrouped))
}

// newAccessedInstance builds an instance that counts as "worked" (non-zero
// LastAccessedAt) so recency ordering is deterministic.
func newAccessedInstance(group, project string, accessed time.Time) *session.Instance {
	return &session.Instance{
		GroupPath:      group,
		ProjectPath:    project,
		LastAccessedAt: accessed,
	}
}

func TestRecentProjectTarget(t *testing.T) {
	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	// Cursor sits in project "alpha"; "beta" was worked more recently than
	// "gamma". Target must be beta's session (most-recent cross-project).
	alpha := newAccessedInstance("alpha/repo", "/p/alpha", base)
	beta := newAccessedInstance("beta/repo", "/p/beta", base.Add(2*time.Hour))
	gamma := newAccessedInstance("gamma/repo", "/p/gamma", base.Add(1*time.Hour))

	h := &Home{
		instances: []*session.Instance{alpha, beta, gamma},
		flatItems: []session.Item{
			{Type: session.ItemTypeSession, Session: alpha},
		},
		cursor: 0,
	}

	target := h.recentProjectTarget()
	assert.NotNil(t, target)
	assert.Equal(t, "g:beta", projectKeyOf(target))

	// Same-project sessions are skipped: with only project "alpha" present,
	// there is no other project to switch to.
	hSingle := &Home{
		instances: []*session.Instance{alpha},
		flatItems: []session.Item{
			{Type: session.ItemTypeSession, Session: alpha},
		},
		cursor: 0,
	}
	assert.Nil(t, hSingle.recentProjectTarget())
}
