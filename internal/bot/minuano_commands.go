package bot

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
)

// handleProjectCommand binds a topic to a Minuano project.
func (b *Bot) handleProjectCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)

	projectName := strings.TrimSpace(msg.CommandArguments())
	if projectName == "" {
		// Show current binding
		threadIDStr := strconv.Itoa(threadID)
		if proj, ok := b.state.GetProject(threadIDStr); ok {
			b.reply(chatID, threadID, fmt.Sprintf("Current project: %s", proj))
		} else {
			b.reply(chatID, threadID, "No project bound. Usage: /project <name>")
		}
		return
	}

	threadIDStr := strconv.Itoa(threadID)
	b.state.BindProject(threadIDStr, projectName)
	b.saveState()
	b.reply(chatID, threadID, fmt.Sprintf("Bound to project: %s", projectName))
}

// handleTasksCommand shows tasks for the bound project with clickable pick buttons.
func (b *Bot) handleTasksCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)
	threadIDStr := strconv.Itoa(threadID)

	project, ok := b.state.GetProject(threadIDStr)
	if !ok {
		b.reply(chatID, threadID, "No project bound. Use /project <name> first.")
		return
	}

	tasks, err := b.minuanoBridge.Status(project)
	if err != nil {
		log.Printf("Error getting tasks for project %s: %v", project, err)
		b.reply(chatID, threadID, "Error: failed to get tasks.")
		return
	}

	if len(tasks) == 0 {
		b.reply(chatID, threadID, fmt.Sprintf("No tasks for project: %s", project))
		return
	}

	// Build text summary
	var lines []string
	lines = append(lines, fmt.Sprintf("Tasks [%s]:", project))
	for _, t := range tasks {
		sym := statusSymbol(t.Status)
		claimedBy := ""
		if t.ClaimedBy != nil {
			claimedBy = fmt.Sprintf(" (%s)", *t.ClaimedBy)
		}
		lines = append(lines, fmt.Sprintf("  %s %s — %s [%s]%s",
			sym, t.ID, t.Title, t.Status, claimedBy))
	}

	// Add inline keyboard buttons for actionable tasks
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, t := range tasks {
		if t.Status != "ready" && t.Status != "pending" {
			continue
		}
		label := fmt.Sprintf("%s %s", statusSymbol(t.Status), truncate(t.Title, 35))
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, "tpick_pick:"+t.ID),
		))
	}

	if len(rows) > 0 {
		kb := tgbotapi.InlineKeyboardMarkup{InlineKeyboard: rows}
		userID := msg.From.ID
		sent, err := b.sendMessageWithKeyboard(chatID, threadID, strings.Join(lines, "\n"), kb)
		if err != nil {
			log.Printf("Error sending tasks with keyboard: %v", err)
			return
		}
		b.mu.Lock()
		b.taskPickerStates[userID] = &taskPickerState{
			Tasks:     tasks,
			Mode:      "pick",
			ChatID:    chatID,
			ThreadID:  threadID,
			MessageID: sent.MessageID,
		}
		b.mu.Unlock()
	} else {
		b.reply(chatID, threadID, strings.Join(lines, "\n"))
	}
}

// handlePickCommand sends a single-task prompt to Claude.
// Supports: /pick (shows task list), /pick <full-id>, /pick <partial-id>
func (b *Bot) handlePickCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)

	partialID := strings.TrimSpace(msg.CommandArguments())

	task, ok := b.resolveTaskID(msg, partialID, "pick")
	if !ok {
		return // picker shown or error sent
	}

	windowID, bound := b.resolveWindow(msg)
	if !bound {
		b.reply(chatID, threadID, "Topic not bound to a session.")
		return
	}

	prompt, err := b.minuanoBridge.PromptSingle(task.ID)
	if err != nil {
		log.Printf("Error generating single prompt for %s: %v", task.ID, err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	if err := b.sendPromptToTmux(windowID, prompt); err != nil {
		if tmux.IsWindowDead(err) {
			b.handleDeadWindow(msg, windowID, "")
			return
		}
		log.Printf("Error sending prompt to tmux: %v", err)
		b.reply(chatID, threadID, "Error: failed to send prompt.")
		return
	}

	b.reply(chatID, threadID, fmt.Sprintf("Working on task %s...", task.ID))
}

// handleAutoCommand sends a loop prompt for autonomous task processing.
func (b *Bot) handleAutoCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)
	threadIDStr := strconv.Itoa(threadID)

	project, ok := b.state.GetProject(threadIDStr)
	if !ok {
		b.reply(chatID, threadID, "No project bound. Use /project <name> first.")
		return
	}

	windowID, bound := b.resolveWindow(msg)
	if !bound {
		b.reply(chatID, threadID, "Topic not bound to a session.")
		return
	}

	prompt, err := b.minuanoBridge.PromptAuto(project)
	if err != nil {
		log.Printf("Error generating auto prompt for %s: %v", project, err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	if err := b.sendPromptToTmux(windowID, prompt); err != nil {
		if tmux.IsWindowDead(err) {
			b.handleDeadWindow(msg, windowID, "")
			return
		}
		log.Printf("Error sending prompt to tmux: %v", err)
		b.reply(chatID, threadID, "Error: failed to send prompt.")
		return
	}

	b.reply(chatID, threadID, fmt.Sprintf("Starting autonomous mode for project %s...", project))
}

// handleBatchCommand sends a multi-task prompt.
func (b *Bot) handleBatchCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)

	args := strings.Fields(msg.CommandArguments())
	if len(args) == 0 {
		b.reply(chatID, threadID, "Usage: /batch <id1> [id2] ...")
		return
	}

	windowID, bound := b.resolveWindow(msg)
	if !bound {
		b.reply(chatID, threadID, "Topic not bound to a session.")
		return
	}

	prompt, err := b.minuanoBridge.PromptBatch(args...)
	if err != nil {
		log.Printf("Error generating batch prompt: %v", err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	if err := b.sendPromptToTmux(windowID, prompt); err != nil {
		if tmux.IsWindowDead(err) {
			b.handleDeadWindow(msg, windowID, "")
			return
		}
		log.Printf("Error sending prompt to tmux: %v", err)
		b.reply(chatID, threadID, "Error: failed to send prompt.")
		return
	}

	b.reply(chatID, threadID, fmt.Sprintf("Working on batch: %s...", strings.Join(args, ", ")))
}

// sendPromptToTmux writes a prompt to a temp file and sends a reference to tmux.
// Long prompts exceed tmux send-keys limits, so we use a temp file.
func (b *Bot) sendPromptToTmux(windowID, prompt string) error {
	// Write prompt to temp file
	tmpFile, err := os.CreateTemp("", "tramuntana-task-*.md")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(prompt); err != nil {
		return fmt.Errorf("writing prompt: %w", err)
	}
	tmpFile.Close()

	// Send reference to tmux
	ref := fmt.Sprintf("Please read and follow the instructions in %s", tmpFile.Name())
	return tmux.SendKeysWithDelay(b.config.TmuxSessionName, windowID, ref, 500)
}

// buildMinuanoEnv returns environment variables to set in tmux windows for Minuano
// integration. Returns nil if MINUANO_DB is not configured.
func (b *Bot) buildMinuanoEnv(windowName string) map[string]string {
	if b.config.MinuanoDB == "" {
		return nil
	}

	env := map[string]string{
		"DATABASE_URL": b.config.MinuanoDB,
		"AGENT_ID":     fmt.Sprintf("tramuntana-%s", windowName),
	}

	if b.config.MinuanoScriptsDir != "" {
		env["PATH"] = fmt.Sprintf("$PATH:%s", b.config.MinuanoScriptsDir)
	}

	return env
}

// statusSymbol returns a display symbol for a task status.
func statusSymbol(status string) string {
	switch status {
	case "pending":
		return "○"
	case "ready":
		return "◎"
	case "claimed":
		return "●"
	case "done":
		return "✓"
	case "failed":
		return "✗"
	default:
		return "?"
	}
}
