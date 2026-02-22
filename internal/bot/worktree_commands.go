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
// Supports: /pickw (shows task list), /pickw <full-id>, /pickw <partial-id>
func (b *Bot) handlePickwCommand(msg *tgbotapi.Message) {
	partialID := strings.TrimSpace(msg.CommandArguments())

	task, ok := b.resolveTaskID(msg, partialID, "pickw")
	if !ok {
		return // picker shown or error sent
	}

	b.executePickwTask(msg.Chat.ID, getThreadID(msg), msg.From.ID, task.ID)
}

// executePickwTask runs the /pickw logic for a resolved task ID.
func (b *Bot) executePickwTask(chatID int64, threadID int, userID int64, taskID string) {
	threadIDStr := strconv.Itoa(threadID)
	userIDStr := strconv.FormatInt(userID, 10)

	project, ok := b.state.GetProject(threadIDStr)
	if !ok {
		b.reply(chatID, threadID, "No project bound. Use /project <name> first.")
		return
	}

	repoRoot, err := b.getRepoRoot(userIDStr, threadIDStr)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	baseBranch, err := git.CurrentBranch(repoRoot)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error getting branch: %v", err))
		return
	}

	branch := fmt.Sprintf("minuano/%s-%s", project, taskID)
	worktreeDir := filepath.Join(repoRoot, ".minuano", "worktrees", fmt.Sprintf("%s-%s", project, taskID))

	b.reply(chatID, threadID, fmt.Sprintf("Creating worktree for %s...", taskID))

	if err := git.WorktreeAdd(repoRoot, worktreeDir, branch); err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error creating worktree: %v", err))
		return
	}

	topicName := fmt.Sprintf("%s [%s]", taskID, project)
	newThreadID, err := b.createForumTopic(chatID, topicName)
	if err != nil {
		git.WorktreeRemove(repoRoot, worktreeDir)
		git.DeleteBranch(repoRoot, branch)
		b.reply(chatID, threadID, fmt.Sprintf("Error creating topic: %v", err))
		return
	}

	env := b.buildMinuanoEnv(fmt.Sprintf("%s-%s", project, taskID))
	windowID, err := tmux.NewWindow(b.config.TmuxSessionName, taskID, worktreeDir, b.config.ClaudeCommand, env)
	if err != nil {
		git.WorktreeRemove(repoRoot, worktreeDir)
		git.DeleteBranch(repoRoot, branch)
		b.reply(chatID, threadID, fmt.Sprintf("Error creating window: %v", err))
		return
	}

	b.waitForSessionMap(windowID)

	newThreadIDStr := strconv.Itoa(newThreadID)
	b.state.BindThread(userIDStr, newThreadIDStr, windowID)
	b.state.SetGroupChatID(userIDStr, newThreadIDStr, chatID)
	b.state.BindProject(newThreadIDStr, project)

	b.state.SetWorktreeInfo(newThreadIDStr, state.WorktreeInfo{
		WorktreeDir: worktreeDir,
		Branch:      branch,
		RepoRoot:    repoRoot,
		BaseBranch:  baseBranch,
		TaskID:      taskID,
	})
	b.saveState()

	prompt, err := b.minuanoBridge.PromptSingle(taskID)
	if err != nil {
		log.Printf("Error generating prompt for %s: %v", taskID, err)
		b.reply(chatID, newThreadID, fmt.Sprintf("Worktree ready but failed to generate prompt: %v", err))
		b.reply(chatID, threadID, fmt.Sprintf("Worktree topic created for %s (branch: %s). Prompt generation failed.", taskID, branch))
		return
	}

	time.Sleep(2 * time.Second)
	if err := b.sendPromptToTmux(windowID, prompt); err != nil {
		log.Printf("Error sending prompt to worktree session: %v", err)
		b.reply(chatID, newThreadID, "Worktree ready but failed to send prompt.")
	}

	b.reply(chatID, threadID, fmt.Sprintf("Worktree topic created for %s (branch: %s)", taskID, branch))
}

// getRepoRoot returns the git repo root for the current window's CWD.
// If the CWD itself is not a git repo, it tries CWD/<project> as a fallback.
func (b *Bot) getRepoRoot(userIDStr, threadIDStr string) (string, error) {
	windowID, bound := b.state.GetWindowForThread(userIDStr, threadIDStr)
	if !bound {
		return "", fmt.Errorf("topic not bound to a session")
	}
	ws, ok := b.state.GetWindowState(windowID)
	if !ok || ws.CWD == "" {
		return "", fmt.Errorf("no CWD known for current session")
	}

	// Try CWD directly
	root, err := git.RepoRoot(ws.CWD)
	if err == nil {
		return root, nil
	}

	// Fallback: try CWD/<project> (e.g. /home/user/code/terminal-game)
	if project, ok := b.state.GetProject(threadIDStr); ok {
		projectDir := filepath.Join(ws.CWD, project)
		if root, err := git.RepoRoot(projectDir); err == nil {
			return root, nil
		}
	}

	return "", fmt.Errorf("git rev-parse --show-toplevel in %s: not a git repository", ws.CWD)
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
