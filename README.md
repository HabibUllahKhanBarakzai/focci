# agent-focus

Bring your terminal (or editor) back to the front the moment an AI coding agent
needs you — and **only** then.

When you kick off a long task in [Claude Code](https://claude.com/claude-code)
(or, soon, [Codex](https://developers.openai.com/codex)) you tab away to do
something else. `agent-focus` refocuses your terminal the instant the agent
finishes its turn or asks for a permission, so you don't have to keep checking.
Crucially, it **ignores the repeating "waiting for your input" idle reminders**,
so once you've come back and moved on it won't keep yanking you back.

> macOS only. Focus-stealing is inherently OS-specific; agent-focus uses the
> macOS app-activation APIs (`open -b` / AppleScript).

## What triggers a refocus

| Agent       | Event                                             | Refocus? |
|-------------|---------------------------------------------------|----------|
| Claude Code | `Stop` (turn finished)                            | ✅ yes   |
| Claude Code | `Notification` — "Claude needs your permission…"  | ✅ yes   |
| Claude Code | `Notification` — "Claude is waiting for your input" (idle reminder) | ❌ ignored |
| Codex       | `notify` — `agent-turn-complete`                  | ✅ yes   |

A short **debounce** (default 1500 ms, per target app) collapses bursts — e.g. a
`Stop` plus a permission `Notification` from the same turn — into a single
refocus.

## Install

### Homebrew (recommended)

```sh
# Before a tagged release exists, build from main:
brew install --HEAD habib-stellic/agent-focus/agent-focus

# Once v0.1.0 is published:
brew install habib-stellic/agent-focus/agent-focus
```

That installs the `agent-focus` binary. Then wire it into your agents:

```sh
agent-focus install            # configures Claude Code (and Codex if present)
```

### From source

```sh
git clone https://github.com/habib-stellic/agent-focus
cd agent-focus
cargo install --path .
agent-focus install
```

## Usage

```
agent-focus install [--agent claude|codex|all] [--command <path>] [--force]
agent-focus uninstall [--agent claude|codex|all]
agent-focus doctor          # show detected app + integration status
agent-focus focus [--force] # refocus right now (handy for testing)
```

- **`install`** adds the hooks to `~/.claude/settings.json` and, for Codex, sets
  `notify` in `~/.codex/config.toml`. It is idempotent, backs up each file to
  `*.bak` before writing, and never overwrites an unrelated Codex `notify`
  unless you pass `--force`.
- **`doctor`** prints which app it would focus and whether the hooks are wired —
  run it first if something isn't working.

### What `install` writes

`~/.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      { "hooks": [ { "type": "command", "command": "/opt/homebrew/bin/agent-focus claude --event stop" } ] }
    ],
    "Notification": [
      { "matcher": "", "hooks": [ { "type": "command", "command": "/opt/homebrew/bin/agent-focus claude --event notification" } ] }
    ]
  }
}
```

`~/.codex/config.toml`:

```toml
notify = ["/opt/homebrew/bin/agent-focus", "codex"]
```

You can also wire it up by hand if you prefer not to let the tool edit your
config.

## How it finds the right window

The agent runs the hook as a child process, so it inherits the GUI app's
`__CFBundleIdentifier` — the bundle id of whatever launched the session
(PyCharm, Warp, iTerm, Terminal, VS Code, …). agent-focus reads that and
activates the app. If it's missing, it falls back to a `TERM_PROGRAM` mapping.

## Configuration (environment variables)

| Variable                  | Default  | Purpose                                                        |
|---------------------------|----------|----------------------------------------------------------------|
| `AGENT_FOCUS_BUNDLE_ID`   | —        | Force a specific app bundle id (overrides detection).          |
| `AGENT_FOCUS_DEBOUNCE_MS` | `1500`   | Debounce window in milliseconds (per target app).              |
| `AGENT_FOCUS_DEBUG`       | off      | Set to `1` to log decisions/outcomes to stderr.                |

## Troubleshooting

```sh
agent-focus doctor
# Then simulate an event with debug output:
echo '{"hook_event_name":"Stop"}' | AGENT_FOCUS_DEBUG=1 agent-focus claude --event stop
```

- **"No host app detected"** — your shell didn't pass `__CFBundleIdentifier` and
  `TERM_PROGRAM` isn't recognized. Set `AGENT_FOCUS_BUNDLE_ID` (find it with
  `osascript -e 'id of app "iTerm"'`).
- **Focuses too often / not enough** — tune `AGENT_FOCUS_DEBOUNCE_MS`.

## Design notes

agent-focus is purely **observational**: the hook entry points always exit `0`
and never emit a `decision`, so they can't block or fail an agent's turn. See
[`src/event.rs`](src/event.rs) for the refocus/ignore logic and
[`src/focus.rs`](src/focus.rs) for activation + debounce.

## License

[Apache-2.0](LICENSE).
