# Tramuntana

> A wind that clears the horizon and clouds the mind.

A Go application that bridges Telegram group topics to [Claude Code](https://docs.anthropic.com/en/docs/claude-code) sessions via tmux. Each topic maps to a tmux window running its own Claude Code process, giving you a persistent, observable AI coding interface from Telegram.

Inspired by [CCBot](https://github.com/six-ddc/ccbot), rewritten from scratch in Go. Complements [Minuano](https://github.com/maquinista-labs/minuano)'s task scheduling with a Telegram interface for interactive sessions, task coordination, and mobile-friendly agent management.

## How it works

```
1 Telegram Topic = 1 tmux Window = 1 Claude Code process
```

Send a message in a Telegram topic, and Tramuntana routes it to the corresponding Claude Code session. Responses stream back as they appear. A session monitor polls JSONL transcripts, formats tool results, and delivers updates through per-user message queues with flood control.

## Quick start

```bash
go build ./cmd/tramuntana

# Install the Claude Code hook (registers SessionStart callback)
tramuntana hook --install

# Set required env vars
export TELEGRAM_BOT_TOKEN=<token>
export ALLOWED_USERS=<comma-separated-user-ids>

# Start the bot
tramuntana serve
```

Open your Telegram group, create a topic, and send a message. Tramuntana will show a directory browser to pick a working directory, then spawn a Claude Code session in that topic.

## Tramuntana vs Minuano

**Tramuntana** (this tool) is the Telegram interface. Use it for:

- Interactive sessions — chat with Claude Code from your phone
- Per-topic isolation — one Telegram topic = one Claude Code session
- Mobile-friendly — screenshots, inline keyboards, rich formatting
- Task coordination — pick, auto, and batch modes via Telegram commands

**[Minuano](https://github.com/maquinista-labs/minuano)** is the headless engine. Use it for:

- Batch processing — `minuano run --agents 4` and walk away
- CI pipelines — scripted task creation and agent spawning
- Local dev — direct terminal access to agents via `minuano attach`

Both share the same Minuano database. Tramuntana calls Minuano commands under the hood.

## CLI commands

| Command | Description |
|---------|-------------|
| `tramuntana serve` | Start the Telegram bot |
| `tramuntana hook --install` | Install Claude Code SessionStart hook |
| `tramuntana version` | Print version |

**`tramuntana serve`** flags:

| Flag | Description |
|------|-------------|
| `--config <path>` | Path to .env override file |

## Telegram commands

All commands use a namespace prefix: `c_` for Claude/terminal, `p_` for project, `t_` for task execution. Use `/menu` to get an inline keyboard with all commands grouped by category.

### Menu

| Command | Description |
|---------|-------------|
| `/menu` | Show inline keyboard with all commands |

### Claude Code (`c_` — forwarded to Claude)

| Command | Description |
|---------|-------------|
| `/c_clear` | Clear Claude session, reset JSONL tracking |
| `/c_compact` | Compact context |
| `/c_cost` | Show token costs |
| `/c_help` | Show Claude help |
| `/c_memory` | Show Claude memory |
| `/c_esc` | Send Escape key to interrupt Claude |
| `/c_screenshot` | Capture terminal as PNG with navigation keyboard |
| `/c_get` | File browser — navigate filesystem and send files |

### Project (`p_` — Minuano project management)

| Command | Description |
|---------|-------------|
| `/p_bind [name]` | Bind topic to a Minuano project (shows current if no arg, prompts for name) |
| `/p_tasks` | List tasks for the bound project with inline pick buttons |
| `/p_add [title]` | Create a Minuano task (prompts for title if omitted, then priority wizard) |
| `/p_delete [id]` | Delete a Minuano task (shows picker if no arg) |
| `/p_history` | Browse JSONL transcript with pagination |

### Task execution (`t_` — run tasks)

| Command | Description |
|---------|-------------|
| `/t_pick [task-id]` | Single-task mode — claim one task, work it (shows picker if no arg) |
| `/t_pickw [task-id]` | Pick task in isolated git worktree |
| `/t_auto` | Auto mode — loop claiming tasks until queue empty |
| `/t_batch [id1 id2...]` | Batch mode — work through tasks in order (prompts for IDs if omitted) |
| `/t_merge [branch]` | Smart merge with automatic conflict resolution (prompts for branch if omitted) |

### Prompt-then-type

Commands that take arguments (`/p_bind`, `/p_add`, `/t_batch`, `/t_merge`) support a two-step flow: tap the command bare, then type the argument as a normal message. The response is intercepted and never forwarded to the Claude session. Issuing any other `/` command cancels the pending prompt.

**`/t_pickw`** creates a full isolated environment:
1. Git worktree at `.minuano/worktrees/<project>-<taskid>`
2. New branch `minuano/<project>-<taskid>`
3. New forum topic for the task
4. New tmux window in the worktree directory
5. Task prompt sent to the new session

**`/t_merge`** runs in two phases:
1. Attempts clean `--no-ff` merge — if successful, cleans up worktree
2. On conflict — aborts merge, creates a merge topic, spawns Claude with conflict file list and resolution instructions

## Interactive UI

Tramuntana detects Claude Code's interactive prompts (permission requests, plan approval, multi-select questions) and renders them as Telegram inline keyboards with navigation buttons. Updates in-place as the UI changes.

### Screenshot control

`/c_screenshot` renders the terminal as a PNG and provides a control keyboard:

| Button | Action |
|--------|--------|
| `↑` `↓` `←` `→` | Arrow keys |
| `Space` `Tab` `Esc` `Enter` | Key presses |
| `Refresh` | Re-capture and update |

### Bash capture

Prefix a message with `!` to run it as a bash command and capture the output directly in Telegram (up to 3800 chars). Cancellable by sending another message.

## Session monitor

The monitor polls JSONL transcript files every 2 seconds and delivers formatted updates:

- **Text** — Claude's responses, split at 4096-char Telegram limit
- **Tool use** — One-line summaries: `**Read**(file.py)`, `**Bash**(git status)`, etc.
- **Tool results** — Formatted per tool type (line counts, diffs, expandable quotes)
- **Thinking** — Truncated to 500 chars in expandable quote
- **Status line** — Claude's spinner/status extracted from terminal, shown as editable message

Tool results are paired with their tool_use entries across poll cycles and edited in-place.

## Dead session recovery

When a tmux window dies (detected on next `send-keys` failure):

1. Cleans up stale state
2. Auto-recreates the window in the same working directory
3. Restores project binding
4. Sends the pending message to the new session
5. Falls back to directory browser if no CWD is known

## Startup recovery

On `tramuntana serve` startup, the bot reconciles persisted state against live tmux windows:

- Live windows — kept as-is
- Dead windows with matching name — re-resolved to new window ID
- Unresolvable windows — dropped, threads unbound, state cleaned

## Rendering

### MarkdownV2

AST-based pipeline using [goldmark](https://github.com/yuin/goldmark) with a custom Telegram MarkdownV2 renderer. Supports emphasis, bold, strikethrough, code blocks, and expandable quotes. Falls back to plain text on conversion failure.

### Screenshots

ANSI terminal output rendered to PNG via `golang.org/x/image` with embedded fonts (JetBrains Mono + Noto CJK + Symbola). Supports 16-color, 256-color, and 24-bit RGB.

## Message queue

Per-user goroutines with 100-item buffered channels. Features:

- **Merging** — consecutive text messages merged up to 3800 chars
- **In-place editing** — tool results edit their tool_use message
- **Status conversion** — status message repurposed as first content message
- **Flood control** — on Telegram 429: 30-second ban, status messages dropped, content delayed
- **Fallback** — MarkdownV2 errors retry as plain text

## Hook system

`tramuntana hook --install` registers a `SessionStart` hook in `~/.claude/settings.json`. When Claude Code starts a new session, it calls `tramuntana hook` which:

1. Reads session info (session_id, cwd) from stdin
2. Gets tmux pane info from `$TMUX_PANE`
3. Writes to `session_map.json` (atomic read-modify-write with flock)

The monitor uses `session_map.json` to locate JSONL files for each session.

## Environment variables

### Required

| Variable | Description |
|----------|-------------|
| `TELEGRAM_BOT_TOKEN` | Bot token from @BotFather |
| `ALLOWED_USERS` | Comma-separated Telegram user IDs |

### Optional

| Variable | Description | Default |
|----------|-------------|---------|
| `ALLOWED_GROUPS` | Comma-separated Telegram group IDs | — |
| `TRAMUNTANA_DIR` | Config/state directory | `~/.tramuntana` |
| `TMUX_SESSION_NAME` | Tmux session name | `tramuntana` |
| `CLAUDE_COMMAND` | Command to start Claude Code | `claude` |
| `MONITOR_POLL_INTERVAL` | Seconds between JSONL polls | `2.0` |
| `MINUANO_BIN` | Path to minuano binary | `minuano` |
| `MINUANO_DB` | Database URL passed to minuano via `--db` | — |
| `MINUANO_SCRIPTS_DIR` | Path to minuano scripts (added to PATH in windows) | — |

## State files

All stored in `$TRAMUNTANA_DIR` (default `~/.tramuntana`), written atomically (temp + fsync + rename):

| File | Description |
|------|-------------|
| `state.json` | Thread bindings, window states, project bindings, worktree info |
| `session_map.json` | Hook output — maps tmux windows to Claude session IDs and CWDs |
| `monitor_state.json` | JSONL byte offsets per session (resume after restart) |

## Requirements

- Go 1.24+
- tmux
- [Claude Code](https://docs.anthropic.com/en/docs/claude-code) CLI
- A Telegram bot token (from [@BotFather](https://t.me/BotFather))
- Optional: [Minuano](https://github.com/maquinista-labs/minuano) for task coordination

## License

MIT
