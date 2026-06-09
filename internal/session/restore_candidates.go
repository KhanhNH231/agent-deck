package session

// RestoreAction is the per-candidate action chosen on the restore ladder when
// the user accepts the "Restore N previous sessions?" prompt.
//
// The mapping from current RevivalClass is:
//   - ClassAlive   → RestoreNoop    (already healthy; do nothing)
//   - ClassErrored → RestoreRevive  (tmux up, pipe dead; reconnect pipe — non-destructive)
//   - ClassDead    → RestoreRestart (tmux gone/stopped; resume via Restart())
//   - nil instance → RestoreSkip    (row deleted between close and reopen)
type RestoreAction int

const (
	RestoreSkip RestoreAction = iota
	RestoreNoop
	RestoreRevive
	RestoreRestart
)

// IsRestoreCandidate reports whether inst should be offered for restore: it was
// open last time (WasOpen) AND is NOT currently alive. "Alive" reuses the same
// Reviver.Classify check the Enter handler uses to choose attach-vs-restart, so
// a session that is currently running (ClassAlive) is already there and is not
// a candidate, while a WasOpen session that is dead/errored is.
//
// rev must be non-nil. A nil instance is never a candidate.
func IsRestoreCandidate(inst *Instance, rev *Reviver) bool {
	if inst == nil || rev == nil {
		return false
	}
	if !inst.WasOpen {
		return false
	}
	return rev.Classify(inst) != ClassAlive
}

// CountRestoreCandidates returns how many instances in the slice satisfy
// IsRestoreCandidate. Used synchronously when a top-level group is expanded to
// decide whether to show the restore prompt (count > 0).
func CountRestoreCandidates(insts []*Instance, rev *Reviver) int {
	n := 0
	for _, inst := range insts {
		if IsRestoreCandidate(inst, rev) {
			n++
		}
	}
	return n
}

// ClassifyRestoreAction maps a candidate's CURRENT state to the restore action.
// A nil instance (row deleted between close and reopen) maps to RestoreSkip.
func ClassifyRestoreAction(inst *Instance, rev *Reviver) RestoreAction {
	if inst == nil || rev == nil {
		return RestoreSkip
	}
	switch rev.Classify(inst) {
	case ClassAlive:
		return RestoreNoop
	case ClassErrored:
		return RestoreRevive
	case ClassDead:
		return RestoreRestart
	default:
		return RestoreSkip
	}
}
