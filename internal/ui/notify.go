package ui

import (
	"sync"
	"time"

	"github.com/asheshgoplani/agent-deck/internal/session"
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

// notifyStableDwell is how long a session must hold a status before it emits an
// iTerm2 notification. Status detection flickers (running↔waiting↔error within
// ~2s — see the flicker_detected warnings); a dwell longer than that flicker
// timescale suppresses transient flips while still alerting on genuinely-stable
// states. Tune if the status-detection flicker window changes.
const notifyStableDwell = 5 * time.Second

// notifyEpisode is one continuous run of a single status for an instance. prev
// is the status held in the immediately-preceding episode, so the reused
// notifyDecision can recognize transition-shaped events (finished = Running→
// Idle) from the stable snapshot. An empty prev marks the baseline episode —
// the status the instance already had when the tracker first saw it — which is
// never notified (it isn't an observed transition; avoids a startup ping storm).
type notifyEpisode struct {
	status session.Status
	prev   session.Status
	since  time.Time
	fired  bool
}

// notifyTracker debounces iTerm2 notifications. It tracks each instance's
// current status episode and authorizes an emit only once that episode has been
// stable for notifyStableDwell, at most once per episode. A flip to a different
// status before the dwell elapses resets the episode, so transient flicker is
// never notified. Methods are mutex-guarded; the status loop is the sole caller
// today, but the lock keeps it safe if that changes.
type notifyTracker struct {
	mu       sync.Mutex
	episodes map[string]*notifyEpisode
}

func newNotifyTracker() *notifyTracker {
	return &notifyTracker{episodes: make(map[string]*notifyEpisode)}
}

// observe records id's current status and returns (true, event) exactly once —
// on the first call where the status has held for >= dwell and notifyDecision
// (prev→cur) classifies it as a notify-worthy, non-attached event. Returns
// (false, "") otherwise: dwell not yet elapsed, already fired this episode,
// attached, disabled, or a baseline episode. now is injected for testability.
func (t *notifyTracker) observe(id string, cur session.Status, isAttached bool, cfg NotifyConfig, now time.Time, dwell time.Duration) (bool, string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ep := t.episodes[id]
	if ep == nil || ep.status != cur {
		prev := session.Status("")
		if ep != nil {
			prev = ep.status
		}
		ep = &notifyEpisode{status: cur, prev: prev, since: now}
		t.episodes[id] = ep
	}
	if ep.fired || isAttached || !cfg.ITermEnabled || ep.prev == "" {
		return false, ""
	}
	if now.Sub(ep.since) < dwell {
		return false, ""
	}
	fire, event := notifyDecision(ep.prev, cur, isAttached, cfg)
	if fire {
		ep.fired = true
	}
	return fire, event
}

// prune drops episodes for instances no longer present (seen[id] == false),
// bounding memory across session churn.
func (t *notifyTracker) prune(seen map[string]bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id := range t.episodes {
		if !seen[id] {
			delete(t.episodes, id)
		}
	}
}
