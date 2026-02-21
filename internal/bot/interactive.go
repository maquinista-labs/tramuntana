package bot

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/monitor"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
)

// interactiveKey identifies an interactive UI session.
type interactiveKey struct {
	UserID   int64
	ThreadID int
}

// interactiveState tracks interactive UI per user+thread.
type interactiveState struct {
	mu       sync.RWMutex
	messages map[interactiveKey]int    // message_id
	modes    map[interactiveKey]string // window_id
}

var interactive = &interactiveState{
	messages: make(map[interactiveKey]int),
	modes:    make(map[interactiveKey]string),
}

// handleInteractiveUI captures pane, detects interactive content, and sends/updates keyboard.
func (b *Bot) handleInteractiveUI(chatID int64, threadID int, userID int64, windowID string) {
	paneText, err := tmux.CapturePane(b.config.TmuxSessionName, windowID, false)
	if err != nil {
		if tmux.IsWindowDead(err) {
			log.Printf("Interactive UI: window %s is dead", windowID)
			clearInteractiveUI(userID, threadID)
		}
		return
	}

	ui, ok := monitor.ExtractInteractiveContent(paneText)
	if !ok {
		return
	}

	keyboard := buildInteractiveKeyboard(ui.Name)
	text := formatInteractiveContent(ui)

	key := interactiveKey{userID, threadID}

	interactive.mu.RLock()
	existingMsgID, hasExisting := interactive.messages[key]
	interactive.mu.RUnlock()

	if hasExisting {
		// Edit existing message
		if err := b.editMessageWithKeyboard(chatID, existingMsgID, text, keyboard); err != nil {
			log.Printf("Error editing interactive message: %v", err)
		}
	} else {
		// Send new message
		msg, err := b.sendMessageWithKeyboard(chatID, threadID, text, keyboard)
		if err != nil {
			log.Printf("Error sending interactive message: %v", err)
			return
		}
		interactive.mu.Lock()
		interactive.messages[key] = msg.MessageID
		interactive.modes[key] = windowID
		interactive.mu.Unlock()
	}
}

// clearInteractiveUI removes the tracked interactive message.
func clearInteractiveUI(userID int64, threadID int) {
	key := interactiveKey{userID, threadID}
	interactive.mu.Lock()
	delete(interactive.messages, key)
	delete(interactive.modes, key)
	interactive.mu.Unlock()
}

// handleInteractiveCallback processes interactive UI navigation callbacks.
func (b *Bot) handleInteractiveCallback(cq *tgbotapi.CallbackQuery) {
	userID := cq.From.ID
	threadID := getThreadID(cq.Message)
	chatID := cq.Message.Chat.ID

	key := interactiveKey{userID, threadID}

	interactive.mu.RLock()
	windowID, ok := interactive.modes[key]
	interactive.mu.RUnlock()

	if !ok {
		return
	}

	data := cq.Data
	session := b.config.TmuxSessionName

	sendKey := func(key string) error {
		return tmux.SendSpecialKey(session, windowID, key)
	}

	var sendErr error
	switch {
	case data == "nav_up":
		sendErr = sendKey("Up")
	case data == "nav_down":
		sendErr = sendKey("Down")
	case data == "nav_left":
		sendErr = sendKey("Left")
	case data == "nav_right":
		sendErr = sendKey("Right")
	case data == "nav_space":
		sendErr = sendKey("Space")
	case data == "nav_tab":
		sendErr = sendKey("Tab")
	case data == "nav_esc":
		sendErr = sendKey("Escape")
		clearInteractiveUI(userID, threadID)
		return
	case data == "nav_enter":
		sendErr = sendKey("Enter")
		clearInteractiveUI(userID, threadID)
		return
	case data == "nav_refresh":
		// Just refresh, no key sent
	default:
		return
	}

	if sendErr != nil {
		if tmux.IsWindowDead(sendErr) {
			log.Printf("Interactive callback: window %s is dead", windowID)
			clearInteractiveUI(userID, threadID)
		}
		return
	}

	// Wait for UI to update, then refresh
	time.Sleep(300 * time.Millisecond)
	b.handleInteractiveUI(chatID, threadID, userID, windowID)
}

// buildInteractiveKeyboard builds the inline keyboard for interactive navigation.
func buildInteractiveKeyboard(uiType string) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton

	if uiType == "RestoreCheckpoint" {
		// Vertical-only layout
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("\u2191", "nav_up"),
				tgbotapi.NewInlineKeyboardButtonData("\u2193", "nav_down"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Enter", "nav_enter"),
				tgbotapi.NewInlineKeyboardButtonData("Esc", "nav_esc"),
				tgbotapi.NewInlineKeyboardButtonData("\U0001F504", "nav_refresh"),
			),
		)
	} else {
		// Full layout
		rows = append(rows,
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("\u2191", "nav_up"),
				tgbotapi.NewInlineKeyboardButtonData("\u2193", "nav_down"),
				tgbotapi.NewInlineKeyboardButtonData("\u2190", "nav_left"),
				tgbotapi.NewInlineKeyboardButtonData("\u2192", "nav_right"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("Space", "nav_space"),
				tgbotapi.NewInlineKeyboardButtonData("Tab", "nav_tab"),
				tgbotapi.NewInlineKeyboardButtonData("Esc", "nav_esc"),
				tgbotapi.NewInlineKeyboardButtonData("Enter", "nav_enter"),
			),
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("\U0001F504 Refresh", "nav_refresh"),
			),
		)
	}

	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

// formatInteractiveContent formats the UI content for display.
func formatInteractiveContent(ui monitor.UIContent) string {
	name := ui.Name
	// Simplify names for display
	if strings.HasPrefix(name, "AskUserQuestion") {
		name = "Question"
	} else if name == "ExitPlanMode" {
		name = "Plan Review"
	} else if name == "PermissionPrompt" {
		name = "Permission"
	} else if name == "RestoreCheckpoint" {
		name = "Restore"
	} else if name == "Settings" {
		name = "Settings"
	}

	content := monitor.ShortenSeparators(ui.Content)
	if len(content) > 3000 {
		content = content[:3000] + "\n..."
	}

	return fmt.Sprintf("[%s]\n%s", name, content)
}
