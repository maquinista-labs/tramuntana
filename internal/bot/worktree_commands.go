package bot

import (
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/git"
	"github.com/otaviocarvalho/tramuntana/internal/state"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
)

// handlePickwCommand creates a worktree + forum topic + Claude session for a task.
func (b *Bot) handlePickwCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)
	threadIDStr := strconv.Itoa(threadID)

	taskID := strings.TrimSpace(msg.CommandArguments())
	if taskID == "" {
		b.reply(chatID, threadID, "Usage: /pickw <task-id>")
		return
	}

	// Require a project binding on the current topic
	project, ok := b.state.GetProject(threadIDStr)
	if !ok {
		b.reply(chatID, threadID, "No project bound. Use /project <name> first.")
		return
	}

	// Get repo root from current window's CWD
	repoRoot, err := b.getRepoRoot(msg)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	// Get current branch as base
	baseBranch, err := git.CurrentBranch(repoRoot)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error getting branch: %v", err))
		return
	}

	branch := fmt.Sprintf("minuano/%s-%s", project, taskID)
	worktreeDir := filepath.Join(repoRoot, ".minuano", "worktrees", fmt.Sprintf("%s-%s", project, taskID))

	b.reply(chatID, threadID, fmt.Sprintf("Creating worktree for %s...", taskID))

	// Create git worktree with new branch
	if err := git.WorktreeAdd(repoRoot, worktreeDir, branch); err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error creating worktree: %v", err))
		return
	}

	// Create forum topic
	topicName := fmt.Sprintf("%s [%s]", taskID, project)
	newThreadID, err := b.createForumTopic(chatID, topicName)
	if err != nil {
		// Clean up on failure
		git.WorktreeRemove(repoRoot, worktreeDir)
		git.DeleteBranch(repoRoot, branch)
		b.reply(chatID, threadID, fmt.Sprintf("Error creating topic: %v", err))
		return
	}

	// Create tmux window in worktree dir
	env := b.buildMinuanoEnv(fmt.Sprintf("%s-%s", project, taskID))
	windowID, err := tmux.NewWindow(b.config.TmuxSessionName, taskID, worktreeDir, b.config.ClaudeCommand, env)
	if err != nil {
		git.WorktreeRemove(repoRoot, worktreeDir)
		git.DeleteBranch(repoRoot, branch)
		b.reply(chatID, threadID, fmt.Sprintf("Error creating window: %v", err))
		return
	}

	// Wait for session_map entry (up to 5s)
	b.waitForSessionMap(windowID)

	// Bind the new thread to the window
	userIDStr := strconv.FormatInt(msg.From.ID, 10)
	newThreadIDStr := strconv.Itoa(newThreadID)
	b.state.BindThread(userIDStr, newThreadIDStr, windowID)
	b.state.SetGroupChatID(userIDStr, newThreadIDStr, chatID)

	// Propagate project binding to the new topic
	b.state.BindProject(newThreadIDStr, project)

	// Store worktree info
	b.state.SetWorktreeInfo(newThreadIDStr, state.WorktreeInfo{
		WorktreeDir: worktreeDir,
		Branch:      branch,
		RepoRoot:    repoRoot,
		BaseBranch:  baseBranch,
		TaskID:      taskID,
	})
	b.saveState()

	// Generate and send task prompt
	prompt, err := b.minuanoBridge.PromptSingle(taskID)
	if err != nil {
		log.Printf("Error generating prompt for %s: %v", taskID, err)
		b.reply(chatID, newThreadID, fmt.Sprintf("Worktree ready but failed to generate prompt: %v", err))
		b.reply(chatID, threadID, fmt.Sprintf("Worktree topic created for %s (branch: %s). Prompt generation failed.", taskID, branch))
		return
	}

	// Wait for Claude to start, then send prompt
	time.Sleep(2 * time.Second)
	if err := b.sendPromptToTmux(windowID, prompt); err != nil {
		log.Printf("Error sending prompt to worktree session: %v", err)
		b.reply(chatID, newThreadID, "Worktree ready but failed to send prompt.")
	}

	b.reply(chatID, threadID, fmt.Sprintf("Worktree topic created for %s (branch: %s)", taskID, branch))
}

// getRepoRoot returns the git repo root for the current window's CWD.
func (b *Bot) getRepoRoot(msg *tgbotapi.Message) (string, error) {
	windowID, bound := b.resolveWindow(msg)
	if !bound {
		return "", fmt.Errorf("topic not bound to a session")
	}
	ws, ok := b.state.GetWindowState(windowID)
	if !ok || ws.CWD == "" {
		return "", fmt.Errorf("no CWD known for current session")
	}
	return git.RepoRoot(ws.CWD)
}

// waitForSessionMap polls for a session_map entry matching the given window ID.
func (b *Bot) waitForSessionMap(windowID string) {
	sessionMapPath := filepath.Join(b.config.TramuntanaDir, "session_map.json")
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		sm, err := state.LoadSessionMap(sessionMapPath)
		if err != nil {
			continue
		}
		for key, entry := range sm {
			if strings.HasSuffix(key, ":"+windowID) {
				b.state.SetWindowState(windowID, state.WindowState{
					SessionID:  entry.SessionID,
					CWD:        entry.CWD,
					WindowName: entry.WindowName,
				})
				b.state.SetWindowDisplayName(windowID, entry.WindowName)
				return
			}
		}
	}
}
