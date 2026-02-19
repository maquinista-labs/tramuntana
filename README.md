# Tramuntana

A wind that clears the horizon and clouds the mind.

Tramuntana bridges Telegram group topics to Claude Code sessions via tmux. Each topic maps to a tmux window running its own Claude Code process, giving you a persistent, observable AI coding interface from Telegram.

Built in Go. Spiritual successor to CCBot.

## How it works

```
1 Telegram Topic = 1 tmux Window = 1 Claude Code process
```

Send a message in a Telegram topic, and Tramuntana routes it to the corresponding Claude Code session. Responses stream back as they appear. Optionally integrates with [Minuano](https://github.com/otaviocc/minuano) for task coordination.

## Usage

```bash
tramuntana serve          # start the bot
tramuntana hook --install # install Claude Code hook
tramuntana version        # print version
```

## Configuration

Set via environment variables:

- `TELEGRAM_BOT_TOKEN` — Telegram bot API token
- `ALLOWED_USERS` — comma-separated Telegram user IDs
- `ALLOWED_GROUPS` — comma-separated Telegram group IDs
