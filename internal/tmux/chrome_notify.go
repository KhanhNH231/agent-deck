//go:build !windows

package tmux

import (
	"io"
	"os"
	"strings"
	"sync"
)

// itermNotifyTtyMu serializes EmitITermNotificationViaTty's /dev/tty writes.
// The status loop emits from a worker pool (errgroup, limit 10), so two or more
// sessions transitioning in the same tick would otherwise open /dev/tty and
// write OSC 9 concurrently — interleaving the escape bytes and garbling both
// notifications (and leaking junk onto the terminal). One short critical section
// per emit keeps each OSC atomic; contention is negligible (a handful of writes
// per tick at most).
var itermNotifyTtyMu sync.Mutex

// iTerm2 OSC 9 "post notification" delivers a macOS Notification Center alert
// to the controlling terminal. Format per iTerm2 docs: ESC ] 9 ; <message> BEL.
//
// This mirrors the SetBadgeFormat (OSC 1337) chrome in chrome.go: agent-deck
// must NOT write these bytes raw to os.Stdout (that would corrupt the running
// Bubbletea TUI frame). The emit paths route through /dev/tty wrapped in a tmux
// DCS passthrough envelope so the OSC reaches the outer iTerm2 whether
// agent-deck runs bare in iTerm or inside a tmux pane.
const (
	itermNotifyOSCPrefix     = "\x1b]9;"
	itermNotifyOSCTerminator = "\a"
)

// iTermNotifyEffective resolves whether the iTerm2 notification feature should
// emit, given the config-sourced default. Mirrors iTermBadgeEffective exactly:
// a single env var named after the config key carries override semantics in
// its value.
//
// Tri-state precedence:
//   - AGENTDECK_ITERM_NOTIFY=1|true|yes|on   → force on, regardless of config.
//   - AGENTDECK_ITERM_NOTIFY=0|false|no|off  → force off, regardless of config.
//   - unset, empty, or unrecognised value    → defer to configEnabled.
//
// Garbage values intentionally fall through to config rather than failing
// closed; users who typo the env var still get their persistent setting.
func iTermNotifyEffective(configEnabled bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTDECK_ITERM_NOTIFY"))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return configEnabled
}

// formatITermNotificationOSC returns the iTerm2 OSC 9 post-notification
// sequence for the given message. Pure formatter — no env / config /
// terminal-detection logic so it stays trivial to test. Both emit paths share
// this byte sequence as their single source of truth.
func formatITermNotificationOSC(message string) string {
	return itermNotifyOSCPrefix + message + itermNotifyOSCTerminator
}

// formatITermNotificationOSCViaTmux wraps formatITermNotificationOSC in a tmux
// DCS passthrough envelope so the OSC reaches the outer terminal even when
// emitted from inside a tmux pane. Mirrors formatITermBadgeOSCViaTmux: the
// inner OSC's lone ESC is doubled and the whole thing is wrapped in
// `ESC P tmux ; <inner> ESC \`.
func formatITermNotificationOSCViaTmux(message string) string {
	inner := strings.ReplaceAll(formatITermNotificationOSC(message), "\x1b", "\x1b\x1b")
	return "\x1bPtmux;" + inner + "\x1b\\"
}

// emitITermNotification writes the iTerm2 OSC 9 notification directly to w.
// Writer-based variant used in tests (bytes.Buffer). Two gates, both must
// permit the emit:
//   - iTerm2Active() — terminal detection (TERM_PROGRAM or LC_TERMINAL).
//   - iTermNotifyEffective(configEnabled) — env-var-or-config decision.
func emitITermNotification(w io.Writer, message string, configEnabled bool) {
	if !iTerm2Active() || !iTermNotifyEffective(configEnabled) {
		return
	}
	_, _ = io.WriteString(w, formatITermNotificationOSC(message))
}

// EmitITermNotificationViaTty writes the OSC 9 notification to /dev/tty wrapped
// in a tmux DCS passthrough envelope. This is the production path: agent-deck's
// status loop runs while the Bubbletea TUI owns os.Stdout, so writing OSC there
// would corrupt the frame. The process's controlling tty is the outer iTerm2
// pty (or the tmux pane's pty when inside tmux), so a wrapped OSC routes out to
// iTerm2 without touching the TUI's stdout.
//
// Silent no-op when:
//   - not iTerm2 (or env / config disabled)
//   - the process has no controlling tty (e.g. detached daemon)
func EmitITermNotificationViaTty(message string, configEnabled bool) {
	if !iTerm2Active() || !iTermNotifyEffective(configEnabled) {
		return
	}

	itermNotifyTtyMu.Lock()
	defer itermNotifyTtyMu.Unlock()

	tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		return
	}
	defer tty.Close()
	_, _ = io.WriteString(tty, formatITermNotificationOSCViaTmux(message))
}
