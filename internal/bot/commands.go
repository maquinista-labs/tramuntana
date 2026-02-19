package bot

import (
	"log"
	"path/filepath"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/state"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
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

// resolveWindow returns the window ID for the user's thread, or empty string if unbound.
func (b *Bot) resolveWindow(msg *tgbotapi.Message) (string, bool) {
	userID := strconv.FormatInt(msg.From.ID, 10)
	threadID := strconv.Itoa(getThreadID(msg))
	return b.state.GetWindowForThread(userID, threadID)
}

// forwardCommand sends a command as text to the bound tmux window.
func (b *Bot) forwardCommand(msg *tgbotapi.Message) {
	windowID, bound := b.resolveWindow(msg)
	if !bound {
		b.reply(msg.Chat.ID, getThreadID(msg), "Topic not bound to a session. Send a message to bind.")
		return
	}

	cmdText := "/" + msg.Command()
	if err := tmux.SendKeysWithDelay(b.config.TmuxSessionName, windowID, cmdText, 500); err != nil {
		log.Printf("Error forwarding command %s to %s: %v", cmdText, windowID, err)
		b.reply(msg.Chat.ID, getThreadID(msg), "Error: failed to send command.")
		return
	}

	// Special handling for /clear: reset session monitoring state
	if msg.Command() == "clear" {
		b.resetSessionTracking(windowID)
	}
}

// resetSessionTracking clears session monitor state for a window after /clear.
func (b *Bot) resetSessionTracking(windowID string) {
	// Remove window state's session info so the monitor starts fresh
	// The monitor_state.json offset will be reset when the new JSONL file appears
	if b.monitorState != nil {
		// Find the session key that matches this window
		sessionMapPath := filepath.Join(b.config.TramuntanaDir, "session_map.json")
		sm, err := loadSessionMapForReset(sessionMapPath)
		if err != nil {
			return
		}
		for key := range sm {
			if windowIDFromKey(key) == windowID {
				b.monitorState.RemoveSession(key)
			}
		}
	}
}

// handleEsc sends Escape key to tmux.
func (b *Bot) handleEsc(msg *tgbotapi.Message) {
	windowID, bound := b.resolveWindow(msg)
	if !bound {
		b.reply(msg.Chat.ID, getThreadID(msg), "Topic not bound to a session.")
		return
	}

	if err := tmux.SendSpecialKey(b.config.TmuxSessionName, windowID, "Escape"); err != nil {
		log.Printf("Error sending Escape to %s: %v", windowID, err)
		b.reply(msg.Chat.ID, getThreadID(msg), "Error: failed to send Escape.")
	}
}

// handleScreenshot captures and sends a terminal screenshot.
func (b *Bot) handleScreenshot(msg *tgbotapi.Message) {
	b.handleScreenshotCommand(msg)
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
// It kills the tmux window and cleans up all related state.
func (b *Bot) handleTopicClose(msg *tgbotapi.Message) {
	threadID := getThreadID(msg)
	threadIDStr := strconv.Itoa(threadID)

	// Find all users bound to this thread and clean up each binding
	cleaned := false
	for _, userID := range b.state.AllUserIDs() {
		windowID, bound := b.state.GetWindowForThread(userID, threadIDStr)
		if !bound {
			continue
		}

		cleaned = true

		// Kill tmux window (ignore errors — may already be dead)
		tmux.KillWindow(b.config.TmuxSessionName, windowID)

		// Clean up state
		b.state.UnbindThread(userID, threadIDStr)
		b.state.RemoveWindowState(windowID)
		b.state.RemoveGroupChatID(userID, threadIDStr)

		// Remove monitor state if available
		if b.monitorState != nil {
			sessionMapPath := filepath.Join(b.config.TramuntanaDir, "session_map.json")
			sm, err := loadSessionMapForReset(sessionMapPath)
			if err == nil {
				for key := range sm {
					if windowIDFromKey(key) == windowID {
						b.monitorState.RemoveSession(key)
						// Also remove from session_map.json
						state.RemoveSessionMapEntry(sessionMapPath, key)
					}
				}
			}
		}
	}

	// Remove project binding for this thread
	b.state.RemoveProject(threadIDStr)

	if cleaned {
		b.saveState()
		log.Printf("Topic %d closed: cleaned up bindings and killed window", threadID)
	}
}

// SetMonitorState sets the monitor state reference (called by serve command).
func (b *Bot) SetMonitorState(ms *state.MonitorState) {
	b.monitorState = ms
}

// loadSessionMapForReset loads session_map.json for the /clear reset logic.
func loadSessionMapForReset(path string) (map[string]state.SessionMapEntry, error) {
	return state.LoadSessionMap(path)
}

// windowIDFromKey extracts the window ID from a session map key ("session:@N" → "@N").
func windowIDFromKey(key string) string {
	if idx := strings.LastIndex(key, ":"); idx >= 0 {
		return key[idx+1:]
	}
	return key
}
