package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleCommand routes slash commands.
func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case "clear", "compact", "cost", "help", "memory":
		b.forwardCommand(msg)
	case "esc":
		b.handleEsc(msg)
	case "screenshot":
		b.handleScreenshot(msg)
	case "history":
		b.handleHistory(msg)
	case "project":
		b.handleProject(msg)
	case "tasks":
		b.handleTasks(msg)
	case "pick":
		b.handlePick(msg)
	case "auto":
		b.handleAuto(msg)
	case "batch":
		b.handleBatch(msg)
	default:
		b.reply(msg.Chat.ID, getThreadID(msg), "Unknown command: /"+msg.Command())
	}
}

// forwardCommand sends a command as text to the bound tmux window.
func (b *Bot) forwardCommand(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 11
	b.reply(msg.Chat.ID, getThreadID(msg), "Command forwarding not yet implemented.")
}

// handleEsc sends Escape key to tmux.
func (b *Bot) handleEsc(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 11
	b.reply(msg.Chat.ID, getThreadID(msg), "/esc not yet implemented.")
}

// handleScreenshot captures and sends a terminal screenshot.
func (b *Bot) handleScreenshot(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 21
	b.reply(msg.Chat.ID, getThreadID(msg), "/screenshot not yet implemented.")
}

// handleHistory shows paginated session history.
func (b *Bot) handleHistory(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 22
	b.reply(msg.Chat.ID, getThreadID(msg), "/history not yet implemented.")
}

// handleProject binds a topic to a Minuano project.
func (b *Bot) handleProject(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 25
	b.reply(msg.Chat.ID, getThreadID(msg), "/project not yet implemented.")
}

// handleTasks shows Minuano tasks.
func (b *Bot) handleTasks(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 25
	b.reply(msg.Chat.ID, getThreadID(msg), "/tasks not yet implemented.")
}

// handlePick picks a Minuano task.
func (b *Bot) handlePick(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 25
	b.reply(msg.Chat.ID, getThreadID(msg), "/pick not yet implemented.")
}

// handleAuto auto-claims a Minuano task.
func (b *Bot) handleAuto(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 25
	b.reply(msg.Chat.ID, getThreadID(msg), "/auto not yet implemented.")
}

// handleBatch runs batch Minuano tasks.
func (b *Bot) handleBatch(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 25
	b.reply(msg.Chat.ID, getThreadID(msg), "/batch not yet implemented.")
}

// handleTopicClose handles forum topic close events.
func (b *Bot) handleTopicClose(msg *tgbotapi.Message) {
	// Placeholder — will be fully implemented in Task 12
}
