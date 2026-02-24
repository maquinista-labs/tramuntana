package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handlePlannerCommand is the entry point for /plan.
func (b *Bot) handlePlannerCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)
	topicIDStr := strconv.Itoa(threadID)

	subcommand := strings.TrimSpace(msg.CommandArguments())
	parts := strings.Fields(subcommand)

	if len(parts) == 0 {
		b.plannerStart(chatID, threadID, topicIDStr, "")
		return
	}

	switch parts[0] {
	case "reopen":
		b.plannerReopen(chatID, threadID, topicIDStr)
	case "release":
		b.plannerRelease(chatID, threadID, topicIDStr)
	case "stop":
		b.plannerStop(chatID, threadID, topicIDStr)
	case "status":
		b.plannerStatus(chatID, threadID, topicIDStr)
	default:
		// Treat the whole argument as the project flag: /plan <project>
		b.plannerStart(chatID, threadID, topicIDStr, parts[0])
	}
}

func (b *Bot) plannerStart(chatID int64, threadID int, topicIDStr, project string) {
	if project == "" {
		project = b.config.DefaultProject
	}
	if project == "" {
		// Try bound project
		project, _ = b.state.GetProject(topicIDStr)
	}
	if project == "" {
		b.reply(chatID, threadID, "No project specified. Use /plan <project> or set TRAMUNTANA_DEFAULT_PROJECT.")
		return
	}

	out, err := b.minuanoBridge.Run("planner", "start", "--topic", topicIDStr, "--project", project)
	if err != nil {
		if strings.Contains(err.Error(), "already running") {
			b.reply(chatID, threadID, "Planner already running here. Use /plan stop first.")
			return
		}
		log.Printf("planner start error: %v", err)
		b.reply(chatID, threadID, fmt.Sprintf("Error starting planner: %v", err))
		return
	}

	_ = out
	b.reply(chatID, threadID, "Planner session started. Send your goals and I will create draft tasks.")
}

func (b *Bot) plannerReopen(chatID int64, threadID int, topicIDStr string) {
	out, err := b.minuanoBridge.Run("planner", "reopen", "--topic", topicIDStr)
	if err != nil {
		log.Printf("planner reopen error: %v", err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}
	_ = out
	b.reply(chatID, threadID, "Planner session reopened.")
}

func (b *Bot) plannerRelease(chatID int64, threadID int, topicIDStr string) {
	project, ok := b.state.GetProject(topicIDStr)
	if !ok {
		project = b.config.DefaultProject
	}
	if project == "" {
		b.reply(chatID, threadID, "No project bound. Use /p_bind first.")
		return
	}

	out, err := b.minuanoBridge.Run("draft-release", "--all", "--project", project)
	if err != nil {
		log.Printf("draft-release error: %v", err)
		b.reply(chatID, threadID, fmt.Sprintf("Error releasing tasks: %v", err))
		return
	}

	// Get tree for confirmation
	tree, _ := b.minuanoBridge.Run("tree", "--project", project)
	msg := strings.TrimSpace(out)
	if tree != "" {
		msg += "\n\n" + strings.TrimSpace(tree)
	}
	b.reply(chatID, threadID, msg)
}

func (b *Bot) plannerStop(chatID int64, threadID int, topicIDStr string) {
	out, err := b.minuanoBridge.Run("planner", "stop", "--topic", topicIDStr)
	if err != nil {
		log.Printf("planner stop error: %v", err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}
	_ = out
	b.reply(chatID, threadID, "Planner session stopped. Draft tasks preserved.")
}

func (b *Bot) plannerStatus(chatID int64, threadID int, topicIDStr string) {
	out, err := b.minuanoBridge.Run("planner", "status")
	if err != nil {
		log.Printf("planner status error: %v", err)
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}
	b.reply(chatID, threadID, strings.TrimSpace(out))
}

// processPlannerCallback handles inline keyboard callbacks from planner crash alerts.
func (b *Bot) processPlannerCallback(cq *tgbotapi.CallbackQuery, data string) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	action, topicIDStr := parts[0], parts[1]

	switch action {
	case "planner_reopen":
		out, err := b.minuanoBridge.Run("planner", "reopen", "--topic", topicIDStr)
		if err != nil {
			b.answerCallback(cq.ID, fmt.Sprintf("Error: %v", err))
			return
		}
		_ = out
		b.answerCallback(cq.ID, "Planner session reopened.")
	}
}
