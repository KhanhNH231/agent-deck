# JIRA Triage Pipeline — assigned-issue queue + digest skill

**Date:** 2026-06-11
**Status:** Approved design
**Placement:** Personal tooling — agent-deck code untouched (spec lives here because
agent-deck is the workflow home base; matches gmail-watcher/meeting-watcher precedent
in `documentation/WATCHERS.md` "Custom external watchers")

## Goal

Get passively notified when a JIRA issue is assigned to me, queue it, and run a
manual `/jira-triage` skill that produces a preliminary digest per issue before
I look at it.

## Locked decisions

| Decision | Choice |
|---|---|
| Trigger | External polling script (launchd, 10 min) — no webhook, no Go adapter |
| Executor | Manual: queue to inbox file, user invokes `/jira-triage` |
| Checks | Issue digest only (no repo mapping, no code recon, no readiness verdict) |
| Poller auth | Atlassian API token in macOS Keychain (`security find-generic-password -s jira-poller -w`) |
| Skill auth | Atlassian MCP (interactive) — two auth surfaces accepted |
| Placement | `~/.config/jira-poller/` (script, plist, state, inbox, digests) + `~/.claude/skills/jira-triage/` |

## Architecture

```
JIRA Cloud ──REST poll──> jira-poller (launchd, 600s) ──append──> inbox.md
                                                            │
user ──/jira-triage──> skill reads inbox ──Atlassian MCP──> digests/KEY-123.md
```

Two artifacts. Zero agent-deck changes.

## Component 1 — Poller (`~/.config/jira-poller/`)

Files: `poll.sh`, `com.khanh.jira-poller.plist` (symlinked into
`~/Library/LaunchAgents/`), `state.json`, `inbox.md`, `poller.log`.

Behavior per tick:

1. Read token: `security find-generic-password -s jira-poller -w`.
   Account/email: `khanh.nguyen@intrepid.asia`. Token never written to log or stdout.
   Site base URL `https://intrepid-asia.atlassian.net` is a non-secret constant at
   the top of `poll.sh`.
2. `GET /rest/api/3/search/jql` (the enhanced endpoint; classic
   `/rest/api/3/search` was removed by Atlassian in 2025). JQL
   `assignee = currentUser() AND statusCategory != Done`, fields
   `summary,status,priority,issuetype`, `maxResults=100`. Token-based
   pagination via `nextPageToken` / `isLast` (no `total` field anymore); loop
   while `isLast=false`, **capped at 5 pages** to defend against the known
   endless-token bug, collecting keys into an in-run Set (dedup within a tick).
3. **Full-list diff** against `state.json` seen-keys map (`key -> first-seen ts`).
   Not a time-window query — laptop sleep cannot drop an assignment.
4. New key → append to `inbox.md`:
   `- [ ] KEY-123 — <summary> (queued 2026-06-11 18:30)`
   and fire `terminal-notifier -title "JIRA" -message "KEY-123 assigned: <summary>"`.
5. Update `state.json` after successful inbox append (append-then-record order, so
   a crash between steps re-appends rather than silently drops; skill tolerates the
   rare duplicate line).

Error handling:

- HTTP/network failure → one log line, exit 0 (launchd retries next tick).
- 401/403 → log + `terminal-notifier` once, gated on error-state transition
  recorded in `state.json` (no notification spam every 10 min).
- Issues reassigned away or closed are NOT removed from `inbox.md`; the skill's
  digest notes current assignee/status.

## Component 2 — Skill (`~/.claude/skills/jira-triage/SKILL.md`)

Invocation: `/jira-triage` (manual, any session with Atlassian MCP available).

Per unchecked `- [ ]` line in `inbox.md`:

1. `getJiraIssue` (+ comments) via Atlassian MCP.
2. Write digest to `~/.config/jira-poller/digests/<KEY>.md`:
   summary, type/priority, reporter, acceptance criteria, epic/linked issues,
   comments gist, current assignee/status (flags reassigned-away).
3. Mark inbox line `[x]`. Duplicate inbox lines for the same key: digest once,
   mark all.
4. End with a one-screen conversation summary of all digests produced.

Error handling: Atlassian MCP unavailable/unauthenticated → report to user,
leave items unchecked (re-runnable).

## Out of scope (add-later compatible — inbox format is the stable interface)

- Conductor/doorbell wiring (`agent-deck session send`)
- Repo/feature mapping, code-level recon, readiness verdict in digests
- First-class `internal/watcher/` jira adapter
- Webhook-based triggering

## Testing / verification

- Poller: manual run with real token; run twice → second run appends nothing
  (diff idempotency); kill between append and state write → next run duplicates
  line, skill handles. 401 path: temporarily wrong token → single notification.
- Skill: run on one real queued issue; verify digest file + inbox `[x]`.
- All claims "not yet verified" until commands run and output captured.

## Risks

- **Two auth surfaces** (Keychain token for poller, MCP session for skill) —
  accepted; MCP expiry fails politely and is re-runnable.
- JIRA REST pagination: the enhanced `/search/jql` endpoint dropped `total` and
  uses `nextPageToken`. Assigned-open set is small (one page in practice); loop on
  `nextPageToken` capped at 5 pages, dedup keys in-run — so the reported
  endless-token bug cannot spam the inbox.
- Tooling deps (verified present): `jq`, `terminal-notifier`, `python3`,
  `security`, `launchctl`. `shellcheck` absent — optional lint, not required.
- launchd + Keychain: first `security find-generic-password` from a launchd job
  may prompt for Keychain access — approve "Always Allow" once during setup.
