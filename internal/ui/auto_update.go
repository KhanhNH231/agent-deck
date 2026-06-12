package ui

import (
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// autoUpdateDue reports whether a feature qualifies for a background ff-pull
// on this tick. It returns false when:
//   - auto-update is disabled (enabled == false)
//   - any of the feature's owning sessions is running or starting
//     (StatusRunning / StatusStarting — activity that may be writing the tree)
//   - the interval has not elapsed since the last attempt for this feature
//
// An empty statuses slice (no sessions for the feature) is treated as safe:
// if enabled and the interval has elapsed, the update is allowed.
func autoUpdateDue(
	enabled bool,
	statuses []session.Status,
	lastAttempt time.Time,
	interval time.Duration,
	now time.Time,
) bool {
	if !enabled {
		return false
	}
	for _, s := range statuses {
		if s == session.StatusRunning || s == session.StatusStarting {
			return false
		}
	}
	if !lastAttempt.IsZero() && now.Sub(lastAttempt) < interval {
		return false
	}
	return true
}
