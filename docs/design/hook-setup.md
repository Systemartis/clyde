# Hook Setup — Connecting Claude Permission Requests to Clyde

Clyde's notification banner can display real-time tool permission requests from
the `claude` CLI. When configured, every `PreToolUse` hook call from Claude
appears in the banner, and you approve or deny it from the keyboard.

## How it works

When `clyde` starts in live mode (without `--demo`):

1. It binds a localhost HTTP server on a random free port (printed to stderr on startup).
2. Each incoming `PreToolUse` hook POST blocks the `claude` CLI until you respond.
3. The notification banner shows: `◆ claude wants to <verb> <arg> in <cwd>`
4. Press **y** to approve, **n** or **Esc** to deny.

## Step 1 — Start clyde

```sh
clyde
```

Look for output like:

```
clyde: hook server on port 49217
clyde: add to ~/.claude/settings.json:
  "hooks": { "PreToolUse": [{ "type": "http", "url": "http://127.0.0.1:49217/hook" }] }
```

The port changes every startup (OS-assigned). You need to update your settings
each time — or see the V2 roadmap below.

## Step 2 — Configure ~/.claude/settings.json

Add (or merge) the following into your `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "type": "http",
        "url": "http://127.0.0.1:PORT/hook"
      }
    ]
  }
}
```

Replace `PORT` with the port printed by clyde at startup.

**Important:** The port is different each time clyde starts. Either:

- Restart clyde, copy the port, update settings before running `claude`.
- Or use a fixed port workaround (not yet implemented — see V2 roadmap).

### Full example settings.json

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "type": "http",
        "url": "http://127.0.0.1:49217/hook"
      }
    ]
  }
}
```

## Step 3 — Keyboard shortcuts in the banner

| Key     | Action                                    |
|---------|-------------------------------------------|
| `y`     | Approve — claude proceeds with the tool   |
| `n`     | Deny — claude is blocked with a message   |
| `Esc`   | Deny + dismiss banner                     |

If you don't respond within the HTTP server's write timeout (~5 minutes), the
claude CLI will time out waiting. Clyde auto-denies pending hooks on quit (`q`).

## Current limitations (V1)

- **Random port**: The port changes every restart. You must manually update
  `settings.json` each time. This is the main friction point.
- **Single pending event**: Clyde shows one event at a time. Concurrent hook
  calls from subagents queue in a buffered channel (capacity 8); if the queue
  fills, clyde auto-approves to avoid blocking claude.
- **PreToolUse only**: PostToolUse and other hook types are not yet consumed.

## V2 roadmap

- **Fixed port or socket file**: Write a well-known port or Unix socket path to
  `~/.config/clyde/hook.port` so the settings.json URL is stable.
- **Auto-write settings.json**: clyde detects when it is not configured and
  offers to patch `~/.claude/settings.json` automatically.
- **PostToolUse**: Show tool results in the banner (errors trigger mascot Surprised).
- **Multi-event queue**: Show a scrollable list when multiple events are pending.
