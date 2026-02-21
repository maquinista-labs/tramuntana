package queue

import (
	"encoding/json"
	"log"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/render"
)

const (
	maxMergeLen = 3800
	chanBufSize = 100
)

// MessageTask represents a message to send to Telegram.
type MessageTask struct {
	UserID      int64
	ThreadID    int
	ChatID      int64
	Parts       []string
	ContentType string // "content", "tool_use", "tool_result", "status_update", "status_clear"
	ToolUseID   string // for tool_result editing
	WindowID    string
}

// userThread is a composite key for per-(user, thread) tracking.
type userThread struct {
	UserID   int64
	ThreadID int
}

// StatusInfo tracks the current status message for a user+thread.
type StatusInfo struct {
	MessageID int
	WindowID  string
	Text      string
}

// Queue manages per-user message sending goroutines.
type Queue struct {
	mu         sync.RWMutex
	api        *tgbotapi.BotAPI
	queues     map[int64]chan MessageTask // user_id → channel
	toolMsgIDs map[string]toolMsgInfo    // tool_use_id → message info
	statusMsgs map[userThread]StatusInfo // (user_id, thread_id) → status message
	flood      *FloodControl
}

type toolMsgInfo struct {
	ChatID    int64
	MessageID int
	ThreadID  int
}

// New creates a new Queue.
func New(api *tgbotapi.BotAPI) *Queue {
	return &Queue{
		api:        api,
		queues:     make(map[int64]chan MessageTask),
		toolMsgIDs: make(map[string]toolMsgInfo),
		statusMsgs: make(map[userThread]StatusInfo),
		flood:      NewFloodControl(),
	}
}

// Enqueue adds a message task to the user's queue.
func (q *Queue) Enqueue(task MessageTask) {
	q.mu.Lock()
	ch, ok := q.queues[task.UserID]
	if !ok {
		ch = make(chan MessageTask, chanBufSize)
		q.queues[task.UserID] = ch
		go q.worker(task.UserID, ch)
	}
	q.mu.Unlock()

	select {
	case ch <- task:
	default:
		log.Printf("Queue full for user %d, dropping message", task.UserID)
	}
}

// QueueLen returns the number of pending messages for a user.
func (q *Queue) QueueLen(userID int64) int {
	q.mu.RLock()
	ch, ok := q.queues[userID]
	q.mu.RUnlock()
	if !ok {
		return 0
	}
	return len(ch)
}

// GetStatusMessage returns the current status message for a user+thread.
func (q *Queue) GetStatusMessage(userID int64, threadID int) (StatusInfo, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	info, ok := q.statusMsgs[userThread{userID, threadID}]
	return info, ok
}

// worker processes messages for a single user.
func (q *Queue) worker(userID int64, ch chan MessageTask) {
	for task := range ch {
		q.processTask(task, ch)
	}
}

func (q *Queue) processTask(task MessageTask, ch chan MessageTask) {
	// Check flood control
	if q.flood.IsFlooded(task.UserID) {
		if task.ContentType == "status_update" || task.ContentType == "status_clear" {
			return // drop status during flood
		}
		q.flood.WaitIfFlooded(task.UserID)
	}

	switch task.ContentType {
	case "content":
		q.processContent(task, ch)
	case "tool_use":
		q.processToolUse(task)
	case "tool_result":
		q.processToolResult(task)
	case "status_update":
		q.processStatusUpdate(task)
	case "status_clear":
		q.processStatusClear(task)
	default:
		q.processContent(task, ch)
	}
}

func (q *Queue) processContent(task MessageTask, ch chan MessageTask) {
	text := strings.Join(task.Parts, "\n")

	// Try to merge consecutive content tasks
	text = q.mergeFromChannel(text, task.WindowID, ch)

	// Try to convert status message to first content message
	ut := userThread{task.UserID, task.ThreadID}
	q.mu.Lock()
	status, hasStatus := q.statusMsgs[ut]
	if hasStatus {
		delete(q.statusMsgs, ut)
	}
	q.mu.Unlock()

	if hasStatus && status.MessageID != 0 {
		// Edit status message in-place with content
		if err := q.editMessage(task.ChatID, status.MessageID, text); err != nil {
			// Fallback: send new message
			q.sendMessage(task.ChatID, task.ThreadID, text)
		}
		return
	}

	q.sendMessage(task.ChatID, task.ThreadID, text)
}

func (q *Queue) processToolUse(task MessageTask) {
	text := strings.Join(task.Parts, "\n")
	msgID := q.sendMessage(task.ChatID, task.ThreadID, text)

	if msgID != 0 && task.ToolUseID != "" {
		q.mu.Lock()
		q.toolMsgIDs[task.ToolUseID] = toolMsgInfo{
			ChatID:    task.ChatID,
			MessageID: msgID,
			ThreadID:  task.ThreadID,
		}
		q.mu.Unlock()
	}
}

func (q *Queue) processToolResult(task MessageTask) {
	text := strings.Join(task.Parts, "\n")

	// Try to edit the tool_use message in-place
	q.mu.Lock()
	info, ok := q.toolMsgIDs[task.ToolUseID]
	if ok {
		delete(q.toolMsgIDs, task.ToolUseID)
	}
	q.mu.Unlock()

	if ok && info.MessageID != 0 {
		if err := q.editMessage(info.ChatID, info.MessageID, text); err != nil {
			// Fallback: send new message
			q.sendMessage(task.ChatID, task.ThreadID, text)
		}
		return
	}

	q.sendMessage(task.ChatID, task.ThreadID, text)
}

func (q *Queue) processStatusUpdate(task MessageTask) {
	text := strings.Join(task.Parts, "\n")
	ut := userThread{task.UserID, task.ThreadID}

	// Send typing indicator when Claude is actively working
	if strings.Contains(strings.ToLower(text), "esc to interrupt") {
		q.sendTyping(task.ChatID)
	}

	q.mu.RLock()
	existing, hasExisting := q.statusMsgs[ut]
	q.mu.RUnlock()

	// Deduplicate: skip if same text
	if hasExisting && existing.Text == text {
		return
	}

	if hasExisting && existing.MessageID != 0 {
		// Edit existing status message
		if err := q.editMessage(task.ChatID, existing.MessageID, text); err == nil {
			q.mu.Lock()
			q.statusMsgs[ut] = StatusInfo{
				MessageID: existing.MessageID,
				WindowID:  task.WindowID,
				Text:      text,
			}
			q.mu.Unlock()
			return
		}
	}

	// Send new status message
	msgID := q.sendMessage(task.ChatID, task.ThreadID, text)
	q.mu.Lock()
	q.statusMsgs[ut] = StatusInfo{
		MessageID: msgID,
		WindowID:  task.WindowID,
		Text:      text,
	}
	q.mu.Unlock()
}

func (q *Queue) processStatusClear(task MessageTask) {
	ut := userThread{task.UserID, task.ThreadID}

	q.mu.Lock()
	status, ok := q.statusMsgs[ut]
	if ok {
		delete(q.statusMsgs, ut)
	}
	q.mu.Unlock()

	if ok && status.MessageID != 0 {
		q.deleteMessage(task.ChatID, status.MessageID)
	}
}

// mergeFromChannel peeks at the channel and merges consecutive content tasks.
func (q *Queue) mergeFromChannel(text, windowID string, ch chan MessageTask) string {
	for {
		select {
		case next, ok := <-ch:
			if !ok {
				return text
			}
			if next.ContentType != "content" || next.WindowID != windowID {
				// Put it back by processing immediately after we're done
				go func() { ch <- next }()
				return text
			}
			nextText := strings.Join(next.Parts, "\n")
			if len(text)+len(nextText)+1 > maxMergeLen {
				go func() { ch <- next }()
				return text
			}
			text = text + "\n" + nextText
		default:
			return text
		}
	}
}

// sendMessage sends a message with MarkdownV2, falling back to plain text.
// Returns the message ID.
func (q *Queue) sendMessage(chatID int64, threadID int, text string) int {
	// Try MarkdownV2 first
	mdv2 := render.ToMarkdownV2(text)
	msgID, err := q.sendRaw(chatID, threadID, mdv2, "MarkdownV2")
	if err == nil {
		return msgID
	}

	// Fallback to plain text
	plain := render.ToPlainText(text)
	msgID, err = q.sendRaw(chatID, threadID, plain, "")
	if err != nil {
		log.Printf("Error sending message: %v", err)
		return 0
	}
	return msgID
}

// sendRaw sends a message via Telegram API.
func (q *Queue) sendRaw(chatID int64, threadID int, text, parseMode string) (int, error) {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	params.AddNonEmpty("text", text)
	if parseMode != "" {
		params.AddNonEmpty("parse_mode", parseMode)
	}
	if threadID != 0 {
		params.AddNonZero("message_thread_id", threadID)
	}

	resp, err := q.api.MakeRequest("sendMessage", params)
	if err != nil {
		q.flood.HandleError(chatID, err)
		return 0, err
	}

	var msg tgbotapi.Message
	json.Unmarshal(resp.Result, &msg)
	return msg.MessageID, nil
}

// editMessage edits a message, trying MarkdownV2 then plain text.
func (q *Queue) editMessage(chatID int64, messageID int, text string) error {
	mdv2 := render.ToMarkdownV2(text)
	err := q.editRaw(chatID, messageID, mdv2, "MarkdownV2")
	if err == nil {
		return nil
	}

	plain := render.ToPlainText(text)
	return q.editRaw(chatID, messageID, plain, "")
}

func (q *Queue) editRaw(chatID int64, messageID int, text, parseMode string) error {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	params.AddNonZero("message_id", messageID)
	params.AddNonEmpty("text", text)
	if parseMode != "" {
		params.AddNonEmpty("parse_mode", parseMode)
	}
	_, err := q.api.MakeRequest("editMessageText", params)
	return err
}

func (q *Queue) deleteMessage(chatID int64, messageID int) {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	params.AddNonZero("message_id", messageID)
	q.api.MakeRequest("deleteMessage", params)
}

// sendTyping sends a "typing" chat action to indicate the bot is working.
func (q *Queue) sendTyping(chatID int64) {
	params := tgbotapi.Params{}
	params.AddNonZero64("chat_id", chatID)
	params.AddNonEmpty("action", "typing")
	q.api.MakeRequest("sendChatAction", params)
}
