//go:build !windows

package tmux

import (
	"os/exec"
)

// iTermBundleID is iTerm2's macOS application bundle identifier. Used twice in
// the terminal-notifier argv: as -sender so the notification adopts iTerm's
// icon/style, and as -activate so a click brings iTerm to the front (alongside
// -execute jumping the TUI to the pane).
const iTermBundleID = "com.googlecode.iterm2"

// buildTerminalNotifierArgs assembles the argv for a CLICKABLE iTerm2
// notification posted via the external `terminal-notifier` CLI. Pure — no
// exec, no env, no terminal detection — so the argv is exhaustively testable.
//
// The click action runs `<binPath> focus <sessionID>` (terminal-notifier's
// -execute runs the value via the shell). binPath and sessionID are kept inside
// the single -execute string; sessionID is internal hex-dash so it carries no
// shell metacharacters, and binPath is the resolved os.Executable() path. Each
// terminal-notifier flag and its value are discrete argv elements, so the
// message/title (which could contain arbitrary session-title text) are never
// concatenated into a shell-interpreted string — they ride -message/-title,
// which terminal-notifier treats as opaque display text.
func buildTerminalNotifierArgs(title, message, sessionID, binPath string) []string {
	return []string{
		"-title", title,
		"-message", message,
		"-sender", iTermBundleID,
		"-activate", iTermBundleID,
		"-execute", binPath + " focus " + sessionID,
	}
}

// terminalNotifierAvailable reports whether the optional `terminal-notifier`
// CLI is on PATH. This is the runtime detection that keeps terminal-notifier an
// OPTIONAL dependency (never a go.mod entry): absent => the caller falls back to
// the OSC 9 path.
func terminalNotifierAvailable() bool {
	_, err := exec.LookPath("terminal-notifier")
	return err == nil
}

// shouldUseClickableNotification is the pure decider the status sweep consults
// to route an emit. The clickable terminal-notifier path is chosen only when
// ALL three gates are open: iTerm2 is the active terminal, the click-action
// config is enabled, and terminal-notifier is on PATH. Any gate closed => false
// and the caller emits the plain OSC 9 notification instead.
func shouldUseClickableNotification(itermActive, clickEnabled, available bool) bool {
	return itermActive && clickEnabled && available
}

// EmitITermNotificationClickable posts a clickable macOS notification via
// terminal-notifier. Fire-and-forget: it uses Cmd.Start() (not Run()) so the
// status loop never blocks on the notifier process, and Wait()s in a detached
// goroutine so the short-lived child is reaped rather than leaked as a zombie.
//
// Gated on iTerm2Active() to match the OSC 9 path — the caller is expected to
// have already consulted shouldUseClickableNotification, but the gate here keeps
// the function safe to call directly. A no-op (no error surface) when iTerm2 is
// not active; the caller owns the fallback decision.
func EmitITermNotificationClickable(title, message, sessionID, binPath string) {
	if !iTerm2Active() {
		return
	}

	args := buildTerminalNotifierArgs(title, message, sessionID, binPath)
	cmd := exec.Command("terminal-notifier", args...)
	if err := cmd.Start(); err != nil {
		return
	}
	// Reap the child without blocking the caller.
	go func() { _ = cmd.Wait() }()
}

// EmitITermNotificationRouted is the single entrypoint the status sweep calls
// for a notify-worthy background transition. It consults the decider and, when
// all gates are open (iTerm active AND clickEnabled AND terminal-notifier on
// PATH), posts a CLICKABLE notification and returns handled=true. Otherwise it
// returns handled=false WITHOUT emitting, signaling the caller to fall back to
// the plain OSC 9 path (EmitITermNotificationViaTty).
//
// Keeping terminal detection and availability behind this one function means
// the UI never imports the unexported gates; it just routes on the bool.
func EmitITermNotificationRouted(title, message, sessionID, binPath string, clickEnabled bool) (handled bool) {
	if !shouldUseClickableNotification(iTerm2Active(), clickEnabled, terminalNotifierAvailable()) {
		return false
	}
	EmitITermNotificationClickable(title, message, sessionID, binPath)
	return true
}
