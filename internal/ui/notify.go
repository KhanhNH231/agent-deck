package ui

import (
	"github.com/asheshgoplani/agent-deck/internal/session"
	"github.com/asheshgoplani/agent-deck/internal/tmux"
)

// Notification event names. Concise wording reused in both the decision result
// and the user-facing message text.
const (
	notifyEventNeedsInput = "needs input"
	notifyEventFinished   = "finished"
	notifyEventError      = "error"
)

// NotifyConfig is the resolved (defaults already applied) on/off state the
// decision function needs. Keeping it a small package-local struct decouples
// the pure decision from the session.NotificationsConfig shape and from any
// env-override logic (which lives at the tmux emit site).
type NotifyConfig struct {
	// ITermEnabled is the master switch for iTerm2 notifications.
	ITermEnabled bool
	// OnNeedsInput / OnFinished / OnError gate each event independently.
	OnNeedsInput bool
	OnFinished   bool
	OnError      bool
}

// notifyConfigFrom maps the user config section to the resolved decision
// config, applying the same nil => true defaults the getters use.
func notifyConfigFrom(n session.NotificationsConfig) NotifyConfig {
	return NotifyConfig{
		ITermEnabled: n.GetITermEnabled(),
		OnNeedsInput: n.GetOnNeedsInput(),
		OnFinished:   n.GetOnFinished(),
		OnError:      n.GetOnError(),
	}
}

// notifyDecision is the pure policy for whether a status transition should emit
// an iTerm2 notification, and which event it is. No terminal / env / IO here so
// it is exhaustively table-testable.
//
// Fires for exactly three BACKGROUND transitions when enabled:
//   - needs-input:   newStatus == Waiting && oldStatus != Waiting
//   - finished/idle: oldStatus == Running && newStatus == Idle
//   - errored:       newStatus == Error && oldStatus != Error
//
// Returns fire=false (event="") when isAttached (the focused pane is noise),
// when iTerm notifications are disabled, when the transition is not one of the
// three, or when that event's per-flag is off.
func notifyDecision(old, new session.Status, isAttached bool, cfg NotifyConfig) (bool, string) {
	if isAttached || !cfg.ITermEnabled {
		return false, ""
	}

	switch {
	case new == session.StatusWaiting && old != session.StatusWaiting:
		if !cfg.OnNeedsInput {
			return false, ""
		}
		return true, notifyEventNeedsInput

	case old == session.StatusRunning && new == session.StatusIdle:
		if !cfg.OnFinished {
			return false, ""
		}
		return true, notifyEventFinished

	case new == session.StatusError && old != session.StatusError:
		if !cfg.OnError {
			return false, ""
		}
		return true, notifyEventError
	}

	return false, ""
}

// notifyMessage builds the user-facing notification text for a session title
// and event. Form: "Agent Deck: <title> needs input" / "<title> finished" /
// "<title> errored".
func notifyMessage(title, event string) string {
	switch event {
	case notifyEventNeedsInput:
		return "Agent Deck: " + title + " needs input"
	case notifyEventFinished:
		return "Agent Deck: " + title + " finished"
	case notifyEventError:
		return "Agent Deck: " + title + " errored"
	}
	return "Agent Deck: " + title
}

// maybeNotifyTransition is the side-effecting wrapper wired into the status
// loop. It computes the decision and, if it fires, emits the OSC 9 via the
// /dev/tty + tmux DCS passthrough path (so the TUI frame on os.Stdout is never
// corrupted). The tmux emitter applies the iTerm2-active gate and the
// AGENTDECK_ITERM_NOTIFY env override; we pass cfg.ITermEnabled as the
// config-sourced default.
//
// No internal throttle: the only caller fires exclusively on an observed
// oldStatus != newStatus change (the status loop's `if newStatus != oldStatus`
// guard), so a steady state cannot produce repeated notifications.
func maybeNotifyTransition(old, new session.Status, title string, isAttached bool, cfg NotifyConfig) {
	fire, event := notifyDecision(old, new, isAttached, cfg)
	if !fire {
		return
	}
	tmux.EmitITermNotificationViaTty(notifyMessage(title, event), cfg.ITermEnabled)
}
