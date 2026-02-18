package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BrowseState holds per-user directory browser state.
type BrowseState struct {
	CurrentPath string
	Page        int
	Dirs        []string
	PendingText string
	MessageID   int
	ChatID      int64
	ThreadID    int
}

// showDirectoryBrowser sends the directory browser keyboard to the user.
func (b *Bot) showDirectoryBrowser(chatID int64, threadID int, userID int64, pendingText string) {
	// Placeholder — will be fully implemented in Task 09
	b.reply(chatID, threadID, "No unbound windows available. Directory browser coming soon.")
}

// processDirectoryCallback handles directory browser callback queries.
func (b *Bot) processDirectoryCallback(cq *tgbotapi.CallbackQuery) {
	// Placeholder — will be fully implemented in Task 09
}
