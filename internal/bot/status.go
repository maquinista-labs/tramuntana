package bot

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

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
	mu           sync.RWMutex
	lastStatus   map[statusKey]string // last status text per user+thread
	pollInterval time.Duration
}

// NewStatusPoller creates a new StatusPoller.
func NewStatusPoller(bot *Bot, q *queue.Queue) *StatusPoller {
	return &StatusPoller{
		bot:          bot,
		queue:        q,
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
				// Status cleared
				sp.mu.Lock()
				delete(sp.lastStatus, key)
				sp.mu.Unlock()

				if sp.queue != nil {
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
