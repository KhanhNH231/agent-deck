package ui

import (
	"log/slog"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/asheshgoplani/agent-deck/internal/session"
)

// resolveAgentDeckBinPath returns the absolute path to the running agent-deck
// binary, falling back to the bare "agent-deck" name (PATH lookup at click
// time) when os.Executable() fails. Embedded in the clickable notification's
// `<bin> focus <id>` action so the click re-invokes THIS binary.
func resolveAgentDeckBinPath() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	return "agent-deck"
}

// focusPollInterval is how often the TUI polls the focus-request handoff file
// written by `agent-deck focus <sessionID>` (the click action of a clickable
// iTerm2 notification). 1s keeps a notification-click feeling responsive
// without adding meaningful load — it is a single stat+read of a tiny file.
const focusPollInterval = time.Second

// focusRequestMsg is delivered to Update when the poller observes a NEW focus
// request (one strictly newer than the last handled). sessionID is the session
// to jump to; requestedAt is its unix-nano timestamp, recorded as
// lastFocusRequestAt so the same request is never handled twice.
type focusRequestMsg struct {
	sessionID   string
	requestedAt int64
}

// focusPollTickMsg drives the re-scheduling poll loop. Carries nothing; the
// handler re-issues focusPoll() to keep ticking (Bubble Tea cmd pattern).
type focusPollTickMsg struct{}

// isNewFocusRequest reports whether a focus request with timestamp requestedAt
// is newer than the last one handled. Strictly-greater so an equal timestamp
// (the request already acted on, or a not-yet-cleared file) does not re-fire.
// Pure — unit-tested in isolation.
func isNewFocusRequest(requestedAt, lastHandled int64) bool {
	return requestedAt > lastHandled
}

// focusPoll returns a tea.Cmd that, after focusPollInterval, reads the
// focus-request file and emits a focusRequestMsg when a NEW request is present
// (timestamp strictly newer than lastFocusRequestAt). It always emits a
// focusPollTickMsg so the loop re-schedules even on no-op ticks. A missing file
// or read error is a silent no-op (the file is the steady-state-absent case).
//
// dir is the agent-deck config dir, resolved once at Init and threaded in so
// the poll closure does no path resolution per tick.
func (h *Home) focusPoll(dir string) tea.Cmd {
	return tea.Tick(focusPollInterval, func(_ time.Time) tea.Msg {
		if dir == "" {
			return focusPollTickMsg{}
		}
		sessionID, requestedAt, ok, err := session.ReadFocusRequest(dir)
		if err != nil {
			// Corrupt file: log once at debug and keep polling. Do NOT act on it.
			notifLog.Debug("focus_request_read_error", slog.String("error", err.Error()))
			return focusPollTickMsg{}
		}
		if !ok || !isNewFocusRequest(requestedAt, h.lastFocusRequestAt) {
			return focusPollTickMsg{}
		}
		return focusRequestMsg{sessionID: sessionID, requestedAt: requestedAt}
	})
}
