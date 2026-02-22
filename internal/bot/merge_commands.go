package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/otaviocarvalho/tramuntana/internal/git"
	"github.com/otaviocarvalho/tramuntana/internal/state"
)

// handleMergeCommand attempts a clean merge; on conflict, spawns a Claude topic.
func (b *Bot) handleMergeCommand(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	threadID := getThreadID(msg)

	branch := strings.TrimSpace(msg.CommandArguments())
	if branch == "" {
		b.reply(chatID, threadID, "Usage: /t_merge <branch>")
		return
	}

	// Get repo root from current window's CWD
	userIDStr := strconv.FormatInt(msg.From.ID, 10)
	threadIDStr := strconv.Itoa(threadID)
	repoRoot, err := b.getRepoRoot(userIDStr, threadIDStr)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error: %v", err))
		return
	}

	// Get current branch as merge target
	baseBranch, err := git.CurrentBranch(repoRoot)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error getting current branch: %v", err))
		return
	}

	b.reply(chatID, threadID, fmt.Sprintf("Merging %s into %s...", branch, baseBranch))

	// Phase 1: try clean merge
	commitMsg := fmt.Sprintf("Merge %s into %s", branch, baseBranch)
	sha, err := git.MergeNoFF(repoRoot, branch, baseBranch, commitMsg)
	if err == nil {
		// Clean merge succeeded
		shortSHA := sha
		if len(sha) > 8 {
			shortSHA = sha[:8]
		}
		b.reply(chatID, threadID, fmt.Sprintf("Merged %s into %s (%s)", branch, baseBranch, shortSHA))

		// Clean up worktree if this branch has one
		b.cleanupWorktreeForBranch(branch)
		return
	}

	// Check if it's a conflict error
	conflictErr, isConflict := err.(*git.ConflictError)
	if !isConflict {
		b.reply(chatID, threadID, fmt.Sprintf("Merge failed: %v", err))
		return
	}

	// Phase 2: conflict — abort and spawn Claude
	if abortErr := git.AbortMerge(repoRoot); abortErr != nil {
		log.Printf("Error aborting merge in %s: %v", repoRoot, abortErr)
	}

	b.reply(chatID, threadID, fmt.Sprintf("Conflict in %d files. Creating merge topic...", len(conflictErr.Files)))

	// Create merge topic
	topicName := fmt.Sprintf("Merge: %s", branch)
	newThreadID, err := b.createForumTopic(chatID, topicName)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error creating merge topic: %v", err))
		return
	}

	// Create tmux window in repo root
	result, err := b.createWindowForDir(repoRoot, msg.From.ID, chatID, newThreadID)
	if err != nil {
		b.reply(chatID, threadID, fmt.Sprintf("Error creating merge session: %v", err))
		return
	}

	// Store merge topic info in state
	newThreadIDStr := strconv.Itoa(newThreadID)
	b.state.SetWorktreeInfo(newThreadIDStr, state.WorktreeInfo{
		RepoRoot:     repoRoot,
		Branch:       branch,
		BaseBranch:   baseBranch,
		IsMergeTopic: true,
	})
	b.saveState()

	// Build conflict resolution prompt
	conflictList := strings.Join(conflictErr.Files, "\n  - ")
	prompt := fmt.Sprintf(`Merge branch %s into %s.

1. Run: git merge --no-ff %s
2. Resolve the conflicts in these files:
  - %s
3. Read both sides of each conflict and understand the intent of each change.
4. Resolve intelligently — don't just pick one side.
5. Run the test suite to verify: go build ./...
6. If tests pass, commit the merge. If not, fix and re-test.
7. When done, say "Merge complete" so I know you're finished.`,
		branch, baseBranch, branch, conflictList)

	// Wait for Claude to start, then send prompt
	time.Sleep(2 * time.Second)
	if err := b.sendPromptToTmux(result.WindowID, prompt); err != nil {
		log.Printf("Error sending merge prompt: %v", err)
		b.reply(chatID, newThreadID, "Session ready but failed to send merge prompt.")
	}

	b.reply(chatID, threadID, "Merge topic created. Claude is resolving conflicts.")
}

// cleanupWorktreeForBranch removes the worktree and branch for a given branch name.
// Called after a successful merge to clean up.
func (b *Bot) cleanupWorktreeForBranch(branch string) {
	for _, threadID := range b.state.AllWorktreeThreadIDs() {
		wi, ok := b.state.GetWorktreeInfo(threadID)
		if !ok || wi.Branch != branch {
			continue
		}
		if wi.WorktreeDir != "" {
			if err := git.WorktreeRemove(wi.RepoRoot, wi.WorktreeDir); err != nil {
				log.Printf("Error removing worktree %s: %v", wi.WorktreeDir, err)
			}
		}
		if err := git.DeleteBranch(wi.RepoRoot, wi.Branch); err != nil {
			log.Printf("Error deleting branch %s: %v", wi.Branch, err)
		}
		b.state.RemoveWorktreeInfo(threadID)
		b.saveState()
		log.Printf("Cleaned up worktree for branch %s (thread %s)", branch, threadID)
		break
	}
}
