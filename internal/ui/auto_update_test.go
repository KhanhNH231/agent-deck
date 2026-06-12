package ui

import (
	"testing"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

var (
	t0       = time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	interval = 30 * time.Minute
)

// TestAutoUpdateDue_Disabled: enabled=false always returns false, regardless of
// statuses or elapsed time.
func TestAutoUpdateDue_Disabled(t *testing.T) {
	got := autoUpdateDue(false, []session.Status{session.StatusIdle}, time.Time{}, interval, t0)
	if got {
		t.Error("disabled: want false, got true")
	}
}

// TestAutoUpdateDue_AnyRunning: a running session blocks the update.
func TestAutoUpdateDue_AnyRunning(t *testing.T) {
	statuses := []session.Status{session.StatusIdle, session.StatusRunning}
	got := autoUpdateDue(true, statuses, time.Time{}, interval, t0)
	if got {
		t.Error("running session present: want false, got true")
	}
}

// TestAutoUpdateDue_AnyStarting: a starting session blocks the update.
func TestAutoUpdateDue_AnyStarting(t *testing.T) {
	got := autoUpdateDue(true, []session.Status{session.StatusStarting}, time.Time{}, interval, t0)
	if got {
		t.Error("starting session present: want false, got true")
	}
}

// TestAutoUpdateDue_TooSoon: interval not elapsed since last attempt.
func TestAutoUpdateDue_TooSoon(t *testing.T) {
	lastAttempt := t0.Add(-15 * time.Minute) // only 15m ago, need 30m
	got := autoUpdateDue(true, []session.Status{session.StatusIdle}, lastAttempt, interval, t0)
	if got {
		t.Error("too-soon: want false, got true")
	}
}

// TestAutoUpdateDue_Due: enabled, all sessions idle/waiting, interval elapsed.
func TestAutoUpdateDue_Due(t *testing.T) {
	lastAttempt := t0.Add(-31 * time.Minute) // 31m ago > 30m interval
	statuses := []session.Status{session.StatusIdle, session.StatusWaiting}
	got := autoUpdateDue(true, statuses, lastAttempt, interval, t0)
	if !got {
		t.Error("due: want true, got false")
	}
}

// TestAutoUpdateDue_NeverAttempted: zero lastAttempt => always due when enabled
// and no running sessions.
func TestAutoUpdateDue_NeverAttempted(t *testing.T) {
	got := autoUpdateDue(true, []session.Status{session.StatusIdle}, time.Time{}, interval, t0)
	if !got {
		t.Error("never attempted: want true, got false")
	}
}

// TestAutoUpdateDue_EmptyStatuses: no sessions for the feature is treated as
// safe; update is allowed when enabled and elapsed.
func TestAutoUpdateDue_EmptyStatuses(t *testing.T) {
	lastAttempt := t0.Add(-31 * time.Minute)
	got := autoUpdateDue(true, nil, lastAttempt, interval, t0)
	if !got {
		t.Error("empty statuses + elapsed: want true, got false")
	}
}

// TestAutoUpdateDue_EmptyStatuses_TooSoon: no sessions, but interval not elapsed.
func TestAutoUpdateDue_EmptyStatuses_TooSoon(t *testing.T) {
	lastAttempt := t0.Add(-5 * time.Minute)
	got := autoUpdateDue(true, nil, lastAttempt, interval, t0)
	if got {
		t.Error("empty statuses + too soon: want false, got true")
	}
}

// TestAutoUpdateDue_NonRunningStatusesAllow: Error/Stopped/Queued sessions do
// NOT block the update — ff-pull only touches clean committed state and a
// crashed/stopped session's worktree benefits from being current when the
// user returns. Only Running/Starting indicate active tree mutation.
func TestAutoUpdateDue_NonRunningStatusesAllow(t *testing.T) {
	lastAttempt := t0.Add(-31 * time.Minute)
	cases := []struct {
		name   string
		status session.Status
	}{
		{"error", session.StatusError},
		{"stopped", session.StatusStopped},
		{"queued", session.StatusQueued},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := autoUpdateDue(true, []session.Status{c.status}, lastAttempt, interval, t0)
			if !got {
				t.Errorf("status %s: want due=true when enabled+elapsed, got false", c.status)
			}
		})
	}
}
