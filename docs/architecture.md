# Architecture

[← Docs index](README.md)

## Components

```mermaid
flowchart LR
    subgraph AGENTS["🤖 Coding agents"]
        direction TB
        CC["🟠 Claude Code"]
        CX["⚫ Codex CLI"]
        OC["🟢 OpenCode"]
    end
    HOOK["🪝 agentpad hook &lt;agent&gt;<br/><sub>300 ms timeout · always exit 0</sub>"]
    subgraph APP["🖥️ agentpad desktop app"]
        direction TB
        RX["📡 Receiver<br/><sub>127.0.0.1:47321</sub>"]
        TR["🧭 Session tracker"]
        LOG[("📜 events.jsonl")]
        UI["🪟 Window + tray"]
        LED["🚦 LED policy"]
        PROF["🎛️ Profiles + editor"]
        W["⚙️ Device worker"]
        RX --> TR --> UI
        RX --> LOG
        TR --> LED --> W
        PROF --> W
    end
    PAD["⌨️ CH57x pad"]
    CC -- hook JSON --> HOOK
    CX -- notify JSON --> HOOK
    OC -- plugin event --> HOOK
    HOOK -- POST --> RX
    W -- "mapping · LED (ch57x-keyboard-tool)" --> PAD
    PAD -. keystrokes .-> AGENTS
    classDef agent fill:#fff4e6,stroke:#f08c00,stroke-width:2px,color:#000
    classDef hook fill:#e7f5ff,stroke:#1c7ed6,stroke-width:2px,color:#000
    classDef app fill:#f3f0ff,stroke:#7048e8,stroke-width:2px,color:#000
    classDef store fill:#fff9db,stroke:#f59f00,stroke-width:2px,color:#000
    classDef pad fill:#ebfbee,stroke:#2f9e44,stroke-width:3px,color:#000
    class CC,CX,OC agent
    class HOOK hook
    class RX,TR,UI,LED,PROF,W app
    class LOG store
    class PAD pad
    style AGENTS fill:#fffaf3,stroke:#f08c00,stroke-dasharray:4 3
    style APP fill:#faf8ff,stroke:#7048e8,stroke-dasharray:4 3
```

## Approval flow

```mermaid
sequenceDiagram
    autonumber
    participant A as 🟠 Claude Code
    participant H as 🪝 agentpad hook
    participant P as 🖥️ agentpad
    participant K as ⌨️ Pad
    A->>H: PermissionRequest (Bash)
    H->>P: POST /v1/events/claude
    P->>P: session → attention
    P->>K: LED "attention" setting 💡
    Note over K: pad lights up
    K->>A: key 1 → "1" (approve once)
    A->>H: PostToolUse
    H->>P: POST /v1/events/claude
    P->>P: session → busy
    P->>K: LED "normal" setting
```

## Event model

Every payload is normalised to an event `{time, agent, session_id, kind, name, tool, cwd, message, raw}`.

| Kind | Session state |
|---|---|
| `session_start` | idle |
| `prompt` | busy |
| `tool_pending` | **attention** |
| `tool_done` | busy |
| `waiting_input` / `idle` | idle |
| `session_end` | removed |
| `other` | unchanged (logged only) |

Aggregate state: **attention** if any session waits for a decision, else **busy** if any works, else **idle**.
It drives the tray icon and the LED policy. Sessions without events for 30 minutes are pruned.

## Receiver API

Loopback only (`127.0.0.1:47321`, non-loopback addresses are refused).

| Method | Path | Body | Response |
|---|---|---|---|
| `POST` | `/v1/events/{claude\|codex\|opencode}` | raw agent payload (≤ 1&nbsp;MiB) | `204` |
| `GET` | `/v1/health` | – | `200 ok` |

## Files

| Path | Content |
|---|---|
| `~/.config/agentpad/profiles/*.yaml` | your profile overrides |
| `~/.local/state/agentpad/events.jsonl` | append-only event log |
| Fyne preferences (`~/.config/fyne/dev.thereisnotime.agentpad/`) | orientation, LED settings, UI state |

## Packages

```text
cmd/agentpad        entry point, `hook` subcommand
internal/config     paths and constants
internal/pad        USB detection, ch57x writer, LED capabilities, LED controller
internal/profile    profiles, validation, store, mapping YAML, orientation
internal/events     normalisation, session tracker, log, HTTP receiver
internal/hook       hook forwarder
internal/setup      agent snippets, udev rule
internal/ui         Fyne desktop UI (only package importing Fyne, besides cmd)
```
