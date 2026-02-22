package bot

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// pendingInput represents a command waiting for user text input.
type pendingInput struct {
	Command  string // "p_bind", "p_add", "t_batch", "t_merge", "t_plan"
	ChatID   int64
	ThreadID int
}

// setPendingInput stores a pending input entry for a user.
func (b *Bot) setPendingInput(userID int64, command string, chatID int64, threadID int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.pendingInputs[userID] = &pendingInput{
		Command:  command,
		ChatID:   chatID,
		ThreadID: threadID,
	}
}

// clearPendingInput removes any pending input for a user.
func (b *Bot) clearPendingInput(userID int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.pendingInputs, userID)
}

// consumePendingInput returns and clears the pending input if it matches the thread.
func (b *Bot) consumePendingInput(userID int64, threadID int) *pendingInput {
	b.mu.Lock()
	defer b.mu.Unlock()
	pi, ok := b.pendingInputs[userID]
	if !ok {
		return nil
	}
	if pi.ThreadID != threadID {
		return nil
	}
	delete(b.pendingInputs, userID)
	return pi
}

// handlePendingInput checks for pending input and dispatches if matched.
// Returns true if the message was consumed (caller must NOT forward to tmux).
func (b *Bot) handlePendingInput(msg *tgbotapi.Message) bool {
	pi := b.consumePendingInput(msg.From.ID, getThreadID(msg))
	if pi == nil {
		return false
	}

	text := msg.Text
	log.Printf("Pending input consumed: command=%s text=%q", pi.Command, text)

	switch pi.Command {
	case "p_bind":
		b.executeProjectBind(msg, text)
	case "p_add":
		b.executeAddWithTitle(msg, text)
	case "t_batch":
		b.executeBatchWithArgs(msg, text)
	case "t_merge":
		b.executeMergeWithBranch(msg, text)
	case "t_plan":
		b.executePlanWithDescription(msg, text)
	default:
		log.Printf("Unknown pending input command: %s", pi.Command)
		return false
	}
	return true
}
