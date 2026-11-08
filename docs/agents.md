# Agent integration

[← Docs index](README.md)

agentpad learns what your agents are doing through their **hooks**. Each hook runs `agentpad hook <agent>`, which forwards
the payload to the running app on `127.0.0.1:47321`. The hook finishes within 300&nbsp;ms and always exits `0`, so it can
never block or break an agent, even when agentpad isn't running.

The **Setup** screen in the app generates these snippets with the absolute path of your binary. agentpad never edits agent configs itself.

## Claude Code

`~/.claude/settings.json` (merge into existing `hooks`):

```json
{
  "hooks": {
    "SessionStart":      [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }],
    "UserPromptSubmit":  [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }],
    "Notification":      [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }],
    "PermissionRequest": [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }],
    "PostToolUse":       [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }],
    "Stop":              [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }],
    "SessionEnd":        [{ "hooks": [{ "type": "command", "command": "agentpad hook claude", "timeout": 5 }] }]
  }
}
```

| Claude event | agentpad state |
|---|---|
| `SessionStart` | session appears (idle) |
| `UserPromptSubmit` | busy |
| `PermissionRequest`, `Notification` with `permission_prompt` | **attention** (LED on) |
| `PostToolUse` | busy (tool approved and ran) |
| `Notification` with `idle_prompt` | idle, waiting for your next prompt |
| `Stop` | idle |
| `SessionEnd` | session removed |

Reference: [Claude Code hooks](https://code.claude.com/docs/en/hooks).

## Codex CLI

`~/.codex/config.toml`:

```toml
notify = ["agentpad", "hook", "codex"]
```

`notify` only reports finished turns (`agent-turn-complete`), so Codex sessions show **busy/idle** but not pending approvals yet.
Codex's newer lifecycle hooks (`hooks.json`) will close that gap.

## OpenCode

Save the plugin from the Setup screen to `~/.config/opencode/plugins/agentpad.js` (or `.opencode/plugins/` in a project).
It forwards bus events with `fetch` and a 300&nbsp;ms timeout.

| OpenCode event | agentpad state |
|---|---|
| `session.created` | session appears |
| `session.status` busy / retry | busy |
| `permission.asked` (older: `permission.updated`) | **attention** |
| `permission.replied` | busy |
| `session.idle`, `session.status` idle | idle |
| `session.deleted` | session removed |

Reference: [OpenCode plugins](https://opencode.ai/docs/plugins/).

## Testing without an agent

```bash
just run          # start the app
just fake-event   # sends a fake Claude permission prompt
just log          # tail the event log
```
