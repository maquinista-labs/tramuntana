package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// addTaskStep represents the current step in the add-task wizard.
type addTaskStep int

const (
	addStepPriority addTaskStep = iota
	addStepBody
)

// addTaskState holds the state for an in-progress task creation wizard.
type addTaskState struct {
	Title     string
	Priority  int
	Project   string
	Step      addTaskStep
	MessageID int
	ChatID    int64
	ThreadID  int
}

// handleAddCommand starts the add-task wizard.
func (b *Bot) handleAddCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)
	userID := msg.From.ID

	title := strings.TrimSpace(msg.CommandArguments())
	if title == "" {
		b.reply(chatID, threadID, "Usage: /p_add <task title>")
		return
	}

	// Resolve project from topic binding
	threadIDStr := strconv.Itoa(threadID)
	project, ok := b.state.GetProject(threadIDStr)
	if !ok {
		b.reply(chatID, threadID, "No project bound. Use /p_bind <name> first.")
		return
	}

	// Show priority keyboard
	text := fmt.Sprintf("New task: %s\n\nSelect priority:", title)
	kb := buildPriorityKeyboard()

	sent, err := b.sendMessageWithKeyboard(chatID, threadID, text, kb)
	if err != nil {
		log.Printf("Error sending priority keyboard: %v", err)
		return
	}

	// Store wizard state
	b.mu.Lock()
	b.addTaskStates[userID] = &addTaskState{
		Title:     title,
		Project:   project,
		Step:      addStepPriority,
		MessageID: sent.MessageID,
		ChatID:    chatID,
		ThreadID:  threadID,
	}
	b.mu.Unlock()
}

// buildPriorityKeyboard returns a 3x3 inline keyboard for priority selection.
func buildPriorityKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("10 (highest)", "task_pri:10"),
			tgbotapi.NewInlineKeyboardButtonData("8", "task_pri:8"),
			tgbotapi.NewInlineKeyboardButtonData("7", "task_pri:7"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("6", "task_pri:6"),
			tgbotapi.NewInlineKeyboardButtonData("5 (default)", "task_pri:5"),
			tgbotapi.NewInlineKeyboardButtonData("4", "task_pri:4"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("3", "task_pri:3"),
			tgbotapi.NewInlineKeyboardButtonData("2", "task_pri:2"),
			tgbotapi.NewInlineKeyboardButtonData("1 (lowest)", "task_pri:1"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "task_cancel"),
		),
	)
}

// buildBodyKeyboard returns the keyboard for the body step (Skip / Cancel).
func buildBodyKeyboard(title string, priority int) (string, tgbotapi.InlineKeyboardMarkup) {
	text := fmt.Sprintf("New task: %s\nPriority: %d\n\nAdd a description? Reply to this message with the body text, or tap Skip.", title, priority)
	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Skip", "task_skip"),
			tgbotapi.NewInlineKeyboardButtonData("Cancel", "task_cancel"),
		),
	)
	return text, kb
}

// buildConfirmationKeyboard returns the final confirmation message with a Pick button.
func buildConfirmationKeyboard(taskID string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Pick this task", "task_pick:"+taskID),
		),
	)
}

// processAddTaskCallback routes task_* callbacks.
func (b *Bot) processAddTaskCallback(cq *tgbotapi.CallbackQuery) {
	data := cq.Data
	userID := cq.From.ID

	switch {
	case strings.HasPrefix(data, "task_pri:"):
		b.handleTaskPriority(cq, userID, data)
	case data == "task_skip":
		b.handleTaskSkipBody(cq, userID)
	case data == "task_cancel":
		b.handleTaskCancel(cq, userID)
	case strings.HasPrefix(data, "task_pick:"):
		b.handleTaskPick(cq, data)
	default:
		log.Printf("Unknown task callback: %s", data)
	}
}

// handleTaskPriority handles priority selection.
func (b *Bot) handleTaskPriority(cq *tgbotapi.CallbackQuery, userID int64, data string) {
	b.mu.Lock()
	ats, ok := b.addTaskStates[userID]
	if !ok {
		b.mu.Unlock()
		return
	}

	priStr := data[len("task_pri:"):]
	priority, err := strconv.Atoi(priStr)
	if err != nil {
		b.mu.Unlock()
		return
	}

	ats.Priority = priority
	ats.Step = addStepBody
	b.mu.Unlock()

	// Update message to body step
	text, kb := buildBodyKeyboard(ats.Title, priority)
	b.editMessageWithKeyboard(ats.ChatID, ats.MessageID, text, kb)
}

// handleTaskSkipBody skips the body and creates the task immediately.
func (b *Bot) handleTaskSkipBody(cq *tgbotapi.CallbackQuery, userID int64) {
	b.mu.RLock()
	ats, ok := b.addTaskStates[userID]
	b.mu.RUnlock()
	if !ok {
		return
	}

	b.createTask(ats, userID, "")
}

// handleTaskCancel cancels the wizard.
func (b *Bot) handleTaskCancel(cq *tgbotapi.CallbackQuery, userID int64) {
	b.mu.Lock()
	ats, ok := b.addTaskStates[userID]
	if ok {
		delete(b.addTaskStates, userID)
	}
	b.mu.Unlock()

	if ok {
		b.editMessageText(ats.ChatID, ats.MessageID, "Task creation cancelled.")
	}
}

// handleTaskPick handles the "Pick this task" button.
// This works independently of active wizard state — the button stays functional.
func (b *Bot) handleTaskPick(cq *tgbotapi.CallbackQuery, data string) {
	taskID := data[len("task_pick:"):]
	chatID := cq.Message.Chat.ID
	threadID := getThreadID(cq.Message)

	// Resolve window using the callback user and thread
	userID := strconv.FormatInt(cq.From.ID, 10)
	threadIDStr := strconv.Itoa(threadID)
	windowID, bound := b.state.GetWindowForThread(userID, threadIDStr)
	if !bound {
		b.reply(chatID, threadID, "Topic not bound to a session. Bind first, then use /pick "+taskID)
		return
	}

	prompt, err := b.minuanoBridge.PromptSingle(taskID)
	if err != nil {
		log.Printf("Error generating single prompt for %s: %v", taskID, err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	if err := b.sendPromptToTmux(windowID, prompt); err != nil {
		log.Printf("Error sending prompt to tmux: %v", err)
		b.reply(chatID, threadID, "Error: failed to send prompt.")
		return
	}

	b.reply(chatID, threadID, fmt.Sprintf("Working on task %s...", taskID))
}

// handleAddTaskReply intercepts text replies to the wizard message for body input.
// Returns true if the message was handled (caller should not process further).
func (b *Bot) handleAddTaskReply(msg *tgbotapi.Message) bool {
	if msg.ReplyToMessage == nil {
		return false
	}

	userID := msg.From.ID

	b.mu.RLock()
	ats, ok := b.addTaskStates[userID]
	b.mu.RUnlock()
	if !ok {
		return false
	}

	// Check that the reply is to the wizard message and we're on the body step
	if msg.ReplyToMessage.MessageID != ats.MessageID {
		return false
	}
	if ats.Step != addStepBody {
		return false
	}

	body := strings.TrimSpace(msg.Text)
	if body == "" {
		return false
	}

	b.createTask(ats, userID, body)
	return true
}

// createTask calls the bridge to create the task and shows the confirmation message.
func (b *Bot) createTask(ats *addTaskState, userID int64, body string) {
	// Clean up wizard state
	b.mu.Lock()
	delete(b.addTaskStates, userID)
	b.mu.Unlock()

	result, err := b.minuanoBridge.Add(ats.Title, ats.Project, body, ats.Priority)
	if err != nil {
		log.Printf("Error creating task: %v", err)
		b.editMessageText(ats.ChatID, ats.MessageID, fmt.Sprintf("Error creating task: %v", err))
		return
	}

	// Build confirmation message
	text := fmt.Sprintf("Created task: %s\n  %s [priority %d]", result.ID, result.Title, ats.Priority)
	if body != "" {
		text += fmt.Sprintf("\n  Body: %s", body)
	}

	kb := buildConfirmationKeyboard(result.ID)
	b.editMessageWithKeyboard(ats.ChatID, ats.MessageID, text, kb)
}
