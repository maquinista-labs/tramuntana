package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
)

// handleTextMessage forwards user text to the bound tmux window.
func (b *Bot) handleTextMessage(msg *tgbotapi.Message) {
	userID := strconv.FormatInt(msg.From.ID, 10)
	threadID := strconv.Itoa(getThreadID(msg))
	chatID := msg.Chat.ID

	// Store group chat ID
	b.state.SetGroupChatID(userID, threadID, chatID)
	b.saveState()

	// Look up window binding
	windowID, bound := b.state.GetWindowForThread(userID, threadID)
	if !bound {
		b.handleUnboundTopic(msg)
		return
	}

	text := msg.Text

	// Handle ! prefix for bash commands
	if strings.HasPrefix(text, "!") && len(text) > 1 {
		b.handleBashCommand(msg, windowID, text)
		return
	}

	// Send text to tmux with 500ms delay before Enter
	if err := tmux.SendKeysWithDelay(b.config.TmuxSessionName, windowID, text, 500); err != nil {
		log.Printf("Error sending keys to %s: %v", windowID, err)
		b.reply(chatID, getThreadID(msg), "Error: failed to send to Claude session.")
	}
}

// handleUnboundTopic shows window picker or directory browser for an unbound topic.
func (b *Bot) handleUnboundTopic(msg *tgbotapi.Message) {
	userID := msg.From.ID
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)

	// Get unbound windows
	windows, err := tmux.ListWindows(b.config.TmuxSessionName)
	if err != nil {
		log.Printf("Error listing windows: %v", err)
		b.reply(chatID, threadID, "Error listing tmux windows.")
		return
	}

	boundWindows := b.state.AllBoundWindowIDs()
	var unboundWindows []tmux.Window
	for _, w := range windows {
		if !boundWindows[w.ID] {
			unboundWindows = append(unboundWindows, w)
		}
	}

	// Store pending text
	pendingText := msg.Text

	if len(unboundWindows) > 0 {
		b.showWindowPicker(chatID, threadID, userID, unboundWindows, pendingText)
	} else {
		b.showDirectoryBrowser(chatID, threadID, userID, pendingText)
	}
}

// handleBashCommand sends a ! command to Claude's bash mode.
func (b *Bot) handleBashCommand(msg *tgbotapi.Message, windowID, text string) {
	session := b.config.TmuxSessionName

	// Send ! first to enter bash mode
	if err := tmux.SendKeys(session, windowID, "!"); err != nil {
		log.Printf("Error sending ! to %s: %v", windowID, err)
		return
	}

	// Wait 1 second
	time.Sleep(1 * time.Second)

	// Send the rest of the command (without !) + Enter
	cmd := text[1:]
	if err := tmux.SendKeysWithDelay(session, windowID, cmd, 500); err != nil {
		log.Printf("Error sending bash command to %s: %v", windowID, err)
	}

	// Bash capture goroutine will be added in Task 23
}

// routeCallback routes callback queries to the appropriate handler.
func (b *Bot) routeCallback(cq *tgbotapi.CallbackQuery) {
	data := cq.Data

	// Answer callback to dismiss spinner
	callback := tgbotapi.NewCallback(cq.ID, "")
	b.api.Request(callback)

	switch {
	case strings.HasPrefix(data, "dir_"):
		b.processDirectoryCallback(cq)
	case strings.HasPrefix(data, "win_"):
		b.processWindowCallback(cq)
	case strings.HasPrefix(data, "hist_"):
		b.handleHistoryCallback(cq)
	case strings.HasPrefix(data, "ss_"):
		b.handleScreenshotCallback(cq)
	case strings.HasPrefix(data, "nav_"):
		b.handleInteractiveCallback(cq)
	default:
		log.Printf("Unknown callback data: %s", data)
	}
}

// handleHistoryCallback is a stub for history pagination callbacks (Task 22).
func (b *Bot) handleHistoryCallback(cq *tgbotapi.CallbackQuery) {
	_ = fmt.Sprintf("history callback: %s", cq.Data)
}

// handleScreenshotCallback is a stub for screenshot callbacks (Task 21).
func (b *Bot) handleScreenshotCallback(cq *tgbotapi.CallbackQuery) {
	_ = fmt.Sprintf("screenshot callback: %s", cq.Data)
}

// handleInteractiveCallback is a stub for interactive UI callbacks (Task 19).
func (b *Bot) handleInteractiveCallback(cq *tgbotapi.CallbackQuery) {
	_ = fmt.Sprintf("interactive callback: %s", cq.Data)
}
