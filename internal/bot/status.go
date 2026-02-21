package bot

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"fmt"

	"github.com/otaviocarvalho/tramuntana/internal/monitor"
	"github.com/otaviocarvalho/tramuntana/internal/queue"
	"github.com/otaviocarvalho/tramuntana/internal/tmux"
)

// statusKey is a composite key for per-(user, thread) status tracking.
type statusKey struct {
	UserID   int64
	ThreadID int
}

// StatusPoller polls Claude's terminal for status line changes and sends updates.
type StatusPoller struct {
	bot          *Bot
	queue        *queue.Queue
	monitor      *monitor.Monitor
	mu           sync.RWMutex
	lastStatus   map[statusKey]string // last status text per user+thread
	pollInterval time.Duration
}

// NewStatusPoller creates a new StatusPoller.
func NewStatusPoller(bot *Bot, q *queue.Queue, mon *monitor.Monitor) *StatusPoller {
	return &StatusPoller{
		bot:          bot,
		queue:        q,
		monitor:      mon,
		lastStatus:   make(map[statusKey]string),
		pollInterval: 1 * time.Second,
	}
}

// Run starts the status polling loop. Blocks until ctx is cancelled.
func (sp *StatusPoller) Run(ctx context.Context) {
	log.Println("Status poller starting...")
	ticker := time.NewTicker(sp.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Status poller stopped.")
			return
		case <-ticker.C:
			sp.poll()
		}
	}
}

func (sp *StatusPoller) poll() {
	// Get all bound window IDs
	boundWindows := sp.bot.state.AllBoundWindowIDs()

	for windowID := range boundWindows {
		// Skip if queue is non-empty for all users of this window (avoid status noise during content delivery)
		users := sp.bot.state.FindUsersForWindow(windowID)
		if len(users) == 0 {
			continue
		}

		// Capture pane (plain text, no ANSI)
		paneText, err := tmux.CapturePane(sp.bot.config.TmuxSessionName, windowID, false)
		if err != nil {
			if tmux.IsWindowDead(err) {
				log.Printf("Status poller: window %s is dead, cleaning up", windowID)
				// Save chat IDs before cleanup removes them
				type notifyTarget struct {
					chatID   int64
					threadID int
				}
				var targets []notifyTarget
				for _, ut := range users {
					if cid, ok := sp.bot.state.GetGroupChatID(ut.UserID, ut.ThreadID); ok {
						tid, _ := strconv.Atoi(ut.ThreadID)
						targets = append(targets, notifyTarget{cid, tid})
					}
				}
				// Clean up UI states for all users on this window
				for _, ut := range users {
					uid, _ := strconv.ParseInt(ut.UserID, 10, 64)
					tid, _ := strconv.Atoi(ut.ThreadID)
					cancelBashCapture(uid, tid)
					clearInteractiveUI(uid, tid)
					// Clear cached status
					sp.mu.Lock()
					delete(sp.lastStatus, statusKey{uid, tid})
					sp.mu.Unlock()
				}
				cleanupDeadWindow(sp.bot, windowID)
				for _, t := range targets {
					sp.bot.reply(t.chatID, t.threadID, "Session died. Send a message to restart.")
				}
			}
			continue
		}

		// Extract status line
		statusText, hasStatus := monitor.ExtractStatusLine(paneText)

		// Update for each observing user
		for _, ut := range users {
			userID, _ := strconv.ParseInt(ut.UserID, 10, 64)
			threadID, _ := strconv.Atoi(ut.ThreadID)
			chatID, ok := sp.bot.state.GetGroupChatID(ut.UserID, ut.ThreadID)
			if !ok {
				continue
			}

			// Skip if queue has pending messages
			if sp.queue != nil && sp.queue.QueueLen(userID) > 0 {
				continue
			}

			key := statusKey{userID, threadID}

			sp.mu.RLock()
			lastText := sp.lastStatus[key]
			sp.mu.RUnlock()

			if hasStatus {
				// Deduplicate: skip if same text
				if statusText == lastText {
					continue
				}

				sp.mu.Lock()
				sp.lastStatus[key] = statusText
				sp.mu.Unlock()

				if sp.queue != nil {
					sp.queue.Enqueue(queue.MessageTask{
						UserID:      userID,
						ThreadID:    threadID,
						ChatID:      chatID,
						Parts:       []string{statusText},
						ContentType: "status_update",
						WindowID:    windowID,
					})
				}
			} else if lastText != "" {
				// Status cleared — Claude finished
				sp.mu.Lock()
				delete(sp.lastStatus, key)
				sp.mu.Unlock()

				// Check for turn timing
				var timingText string
				if sp.monitor != nil {
					if start, ok := sp.monitor.GetAndClearTurnStart(windowID); ok {
						elapsed := time.Since(start)
						timingText = formatDuration(elapsed)
					}
				}

				if sp.queue != nil {
					if timingText != "" {
						// Send timing as content before clearing status
						sp.queue.Enqueue(queue.MessageTask{
							UserID:      userID,
							ThreadID:    threadID,
							ChatID:      chatID,
							Parts:       []string{timingText},
							ContentType: "content",
							WindowID:    windowID,
						})
					}
					sp.queue.Enqueue(queue.MessageTask{
						UserID:      userID,
						ThreadID:    threadID,
						ChatID:      chatID,
						ContentType: "status_clear",
						WindowID:    windowID,
					})
				}
			}
		}
	}
}

// formatDuration formats a duration as "Brewed for Xm Ys" or "Brewed for Ys".
func formatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("Brewed for %ds", secs)
	}
	mins := secs / 60
	secs = secs % 60
	return fmt.Sprintf("Brewed for %dm %ds", mins, secs)
}
