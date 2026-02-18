package bot

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
)

// showWindowPicker sends the window picker keyboard to the user.
func (b *Bot) showWindowPicker(chatID int64, threadID int, userID int64, windows []tmux.Window, pendingText string) {
	// Placeholder — will be fully implemented in Task 10
	b.reply(chatID, threadID, "Unbound windows available. Window picker coming soon.")
}

// processWindowCallback handles window picker callback queries.
func (b *Bot) processWindowCallback(cq *tgbotapi.CallbackQuery) {
	// Placeholder — will be fully implemented in Task 10
}
