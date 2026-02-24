package listener

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
)

// TaskEvent represents a task status change notification.
type TaskEvent struct {
	TaskID    string  `json:"task_id"`
	Title     string  `json:"title"`
	Status    string  `json:"status"`
	OldStatus string  `json:"old_status"`
	ProjectID string  `json:"project_id"`
	AgentID   string  `json:"agent_id"`
	Ts        float64 `json:"ts"`
}

// PlannerEvent represents a planner session status change notification.
type PlannerEvent struct {
	SessionID string `json:"session_id"`
	TopicID   int64  `json:"topic_id"`
	ProjectID string `json:"project_id"`
	Status    string `json:"status"`
	OldStatus string `json:"old_status"`
}

// EventListener listens for Postgres NOTIFY events on task_events and planner_events channels.
type EventListener struct {
	databaseURL   string
	TaskEvents    chan TaskEvent
	PlannerEvents chan PlannerEvent
}

// New creates a new EventListener.
func New(databaseURL string) *EventListener {
	return &EventListener{
		databaseURL:   databaseURL,
		TaskEvents:    make(chan TaskEvent, 64),
		PlannerEvents: make(chan PlannerEvent, 16),
	}
}

// Start begins listening for NOTIFY events. Blocks until context is cancelled.
// Automatically reconnects with exponential backoff on connection errors.
func (l *EventListener) Start(ctx context.Context) error {
	var attempt int
	for {
		err := l.listen(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		attempt++
		delay := backoff(attempt)
		log.Printf("listener: connection lost (%v), reconnecting in %v (attempt %d)", err, delay, attempt)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (l *EventListener) listen(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, l.databaseURL)
	if err != nil {
		return fmt.Errorf("connecting: %w", err)
	}
	defer conn.Close(ctx)

	// Subscribe to channels.
	if _, err := conn.Exec(ctx, "LISTEN task_events"); err != nil {
		return fmt.Errorf("LISTEN task_events: %w", err)
	}
	if _, err := conn.Exec(ctx, "LISTEN planner_events"); err != nil {
		return fmt.Errorf("LISTEN planner_events: %w", err)
	}

	log.Println("listener: connected and listening on task_events, planner_events")

	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("waiting for notification: %w", err)
		}

		switch notification.Channel {
		case "task_events":
			var ev TaskEvent
			if err := json.Unmarshal([]byte(notification.Payload), &ev); err != nil {
				log.Printf("listener: bad task_events payload: %v", err)
				continue
			}
			select {
			case l.TaskEvents <- ev:
			default:
				log.Printf("listener: task_events channel full, dropping event for %s", ev.TaskID)
			}

		case "planner_events":
			var ev PlannerEvent
			if err := json.Unmarshal([]byte(notification.Payload), &ev); err != nil {
				log.Printf("listener: bad planner_events payload: %v", err)
				continue
			}
			select {
			case l.PlannerEvents <- ev:
			default:
				log.Printf("listener: planner_events channel full, dropping event for %s", ev.SessionID)
			}
		}
	}
}

func backoff(attempt int) time.Duration {
	d := time.Duration(math.Min(float64(attempt*attempt), 30)) * time.Second
	if d < time.Second {
		d = time.Second
	}
	return d
}
