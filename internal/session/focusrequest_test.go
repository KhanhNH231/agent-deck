package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestFocusRequest_RoundTrip writes a focus request and reads it back, asserting
// the session id and the requested-at unix-nano timestamp survive the JSON
// round-trip exactly.
func TestFocusRequest_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1717900000, 123456789)

	require.NoError(t, WriteFocusRequest(dir, "sess-abcd-1234", now))

	id, requestedAt, ok, err := ReadFocusRequest(dir)
	require.NoError(t, err)
	require.True(t, ok, "a written focus request must read back ok=true")
	require.Equal(t, "sess-abcd-1234", id)
	require.Equal(t, now.UnixNano(), requestedAt)
}

// TestFocusRequest_MissingFile asserts a read of a directory with no focus
// request returns ok=false and no error (the common steady-state poll case).
func TestFocusRequest_MissingFile(t *testing.T) {
	dir := t.TempDir()

	id, requestedAt, ok, err := ReadFocusRequest(dir)
	require.NoError(t, err, "missing file is not an error")
	require.False(t, ok)
	require.Empty(t, id)
	require.Zero(t, requestedAt)
}

// TestFocusRequest_Clear removes the request file; a subsequent read reports
// ok=false. Clearing an already-absent file is also a no-op (no error).
func TestFocusRequest_Clear(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, WriteFocusRequest(dir, "sess-x", time.Unix(1, 0)))

	require.NoError(t, ClearFocusRequest(dir))

	_, _, ok, err := ReadFocusRequest(dir)
	require.NoError(t, err)
	require.False(t, ok, "request must be gone after Clear")

	require.NoError(t, ClearFocusRequest(dir), "clearing an absent request must be a no-op")
}

// TestFocusRequest_MalformedJSON asserts a corrupt request file surfaces an
// error (ok=false) rather than panicking, so the TUI poller can swallow it.
func TestFocusRequest_MalformedJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, focusRequestFileName)
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0o644))

	require.NotPanics(t, func() {
		_, _, ok, err := ReadFocusRequest(dir)
		require.Error(t, err, "malformed JSON must surface an error")
		require.False(t, ok)
	})
}
