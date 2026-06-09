package session

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// focusRequestFileName is the focus-request handoff file under the agent-deck
// config dir. The `agent-deck focus <sessionID>` subcommand writes it; the
// running TUI polls and clears it. A single file (last-writer-wins) is
// deliberate: only the most recent click matters.
const focusRequestFileName = "focus_request.json"

// focusRequest is the on-disk JSON payload. requested_at_unixnano lets the TUI
// poller distinguish a NEW request from one it has already handled (clicking
// the same notification twice, or a stale file left after a missed clear).
type focusRequest struct {
	SessionID           string `json:"session_id"`
	RequestedAtUnixNano int64  `json:"requested_at_unixnano"`
}

// WriteFocusRequest atomically writes a focus request for sessionID into dir.
// Atomic (temp file + os.Rename) so the polling TUI never observes a
// half-written file. now is injected for testability.
func WriteFocusRequest(dir, sessionID string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	payload, err := json.Marshal(focusRequest{
		SessionID:           sessionID,
		RequestedAtUnixNano: now.UnixNano(),
	})
	if err != nil {
		return err
	}

	path := filepath.Join(dir, focusRequestFileName)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, payload, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// ReadFocusRequest reads the focus request from dir. A missing file is the
// common steady-state and returns ok=false with no error. A present-but-corrupt
// file returns an error (ok=false) so the caller can log-and-skip rather than
// act on garbage. On success returns the sessionID, the requested-at unix-nano,
// and ok=true.
func ReadFocusRequest(dir string) (sessionID string, requestedAt int64, ok bool, err error) {
	path := filepath.Join(dir, focusRequestFileName)
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return "", 0, false, nil
		}
		return "", 0, false, readErr
	}

	var req focusRequest
	if jsonErr := json.Unmarshal(data, &req); jsonErr != nil {
		return "", 0, false, jsonErr
	}
	return req.SessionID, req.RequestedAtUnixNano, true, nil
}

// ClearFocusRequest removes the focus-request file. An absent file is not an
// error (idempotent — the TUI clears after handling, and a missed clear must
// not wedge the poller).
func ClearFocusRequest(dir string) error {
	path := filepath.Join(dir, focusRequestFileName)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
