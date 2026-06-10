# Handoff: Slack app notifications (Feature 3)

**Status:** Not started. Design decisions locked; investigation done. Pick up in a
fresh session.
**Branch:** feat/agent-deck-session-ux

## Goal

Post agent-deck session status events to a Slack channel, so the user gets
notified off-machine when a session needs input / finishes / errors.

## Locked decisions

- **Transport:** Slack **app + bot token** (not an incoming webhook). Post to a
  channel **by name**, **threading per session** (one thread per session; status
  updates land as replies).
- **Events:** reuse the existing three notify events — `needs input`, `finished`,
  `error` — with the same 5s stable-dwell debounce and per-episode de-dupe.
- **Per-event on/off** mirrors the iTerm flags.

## Key finding — reuse the notify policy, add a sink

The notification *policy* is already sink-agnostic and battle-tested in
`internal/ui/notify.go`:
- `notifyDecision(old, new, isAttached, cfg)` → (fire, event)
- `notifyTracker.observe(id, status, isAttached, cfg, now, dwell)` → (fire, event)
  — debounces flicker, emits once per stable episode.
- `notifyMessage(title, event)` → user-facing text.

It is emitted today **only** for iTerm2, in `home.go:3457-3476` (status loop),
gated by `if notifyCfg.ITermEnabled`.

### The wiring change

That `if notifyCfg.ITermEnabled` guard wraps the whole observe-loop. Slack must
fire **independently** of iTerm (user may want Slack but not iTerm). Refactor:

```
if (notifyCfg.ITermEnabled || slackEnabled) && h.notifyTracker != nil {
    for inst := range instances {
        if fire, event := observe(...); fire {
            if notifyCfg.ITermEnabled { ...existing iTerm emit... }
            if slackEnabled { go slackSink.Post(inst, event, message) }  // non-blocking
        }
    }
}
```

`notifyTracker.observe` must be called **once per episode** regardless of how
many sinks consume it — so keep a single observe call and fan out the result.
Do NOT add a second tracker; that would double-debounce.

Slack POST is network I/O — must NOT block the status-loop goroutine. Dispatch
to a bounded worker / channel; drop or log on backpressure (never stall the TUI).

## Threading per session

Slack `chat.postMessage` returns a `ts`; replies set `thread_ts`. Need a
per-session map `sessionID -> slackThreadTs` (in-memory is fine; rebuild lazily).
First event for a session opens the thread; subsequent events reply with the
session title + event. Decide: persist thread ts across restarts? (Probably not
needed — a new thread per run is acceptable. Confirm with user.)

## Config

`internal/session/userconfig.go`:
- `UserConfig` (line 43), `NotificationsConfig` (line 660) with
  `GetITermEnabled/OnNeedsInput/OnFinished/OnError` getters (721-768).
- Add a `Slack` sub-config: `enabled`, `bot_token` (**SECRET** — see below),
  `channel`, and per-event flags (reuse OnNeedsInput/Finished/Error semantics, or
  add slack-scoped ones). Lives in `~/.agent-deck/config.toml`.

### Secret handling (IMPORTANT)

Bot token is a secret. Do **not** log it. Prefer reading from an env var
(`AGENTDECK_SLACK_BOT_TOKEN`) over storing the raw token in `config.toml`; if
stored in toml, never echo it in logs/errors. Honor the user's rule: never peek
into `.env`/secret files. Confirm storage approach with the user before coding.

## Suggested structure

- `internal/notify/slack/` (new pkg): a thin Slack client — `Post(channel,
  message, threadTs) (ts, err)`. SDK-style, single responsibility, mockable at
  the boundary. No conditional logic in tests.
- `internal/ui` wiring: a `slackSink` holding client + thread map + worker;
  called from the status-loop fan-out.
- Reuse `notifyMessage` for text.

## Open questions for the user

1. Persist Slack thread ts across restarts, or fresh thread per session run?
2. Bot token via env var only, or also allow `config.toml`?
3. Channel: single global channel, or per-group/per-project channel override?
4. Setup UX: add to the setup wizard (`internal/ui/setup_wizard.go`) and/or a
   `agent-deck` CLI subcommand to test the connection?

## Files likely touched

- `internal/notify/slack/*.go` (new) + tests
- `internal/session/userconfig.go` (Slack config + getters)
- `internal/ui/home.go` (status-loop fan-out, slackSink field/init)
- `internal/ui/notify.go` (maybe: a `notifySinks` abstraction if it stays clean)
- `internal/ui/setup_wizard.go` (optional setup)
- This is >5 files → **split**: (a) Slack client pkg + config, (b) status-loop
  wiring + threading, (c) setup UX. Brainstorm + spec each per CLAUDE.md.

## Verification plan

- Unit: Slack client request shaping (channel/thread_ts/text) with a mocked HTTP
  boundary; config parse/getters; fan-out fires Slack only when enabled and once
  per episode (extend notify tests).
- Manual: real bot token in a test channel — observe thread creation + replies on
  needs-input / finished / error. Label "not yet verified" until run.
