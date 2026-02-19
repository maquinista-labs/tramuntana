package queue

import (
	"strings"
	"sync"
	"time"
)

// FloodControl handles Telegram 429 rate limiting.
type FloodControl struct {
	mu         sync.RWMutex
	floodUntil map[int64]time.Time // user_id → flood ban expiry
}

// NewFloodControl creates a new FloodControl instance.
func NewFloodControl() *FloodControl {
	return &FloodControl{
		floodUntil: make(map[int64]time.Time),
	}
}

// HandleError checks for 429 errors and sets flood bans.
func (fc *FloodControl) HandleError(chatID int64, err error) {
	if err == nil {
		return
	}
	errStr := err.Error()
	// Telegram 429 errors contain "Too Many Requests" and "retry after"
	if strings.Contains(errStr, "Too Many Requests") || strings.Contains(errStr, "429") {
		// Set a conservative 30-second flood ban
		fc.mu.Lock()
		fc.floodUntil[chatID] = time.Now().Add(30 * time.Second)
		fc.mu.Unlock()
	}
}

// IsFlooded returns true if a user is currently flood-banned.
func (fc *FloodControl) IsFlooded(userID int64) bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	until, ok := fc.floodUntil[userID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		return false
	}
	return true
}

// WaitIfFlooded blocks until the flood ban expires (max 10 seconds).
func (fc *FloodControl) WaitIfFlooded(userID int64) {
	fc.mu.RLock()
	until, ok := fc.floodUntil[userID]
	fc.mu.RUnlock()

	if !ok {
		return
	}

	remaining := time.Until(until)
	if remaining <= 0 {
		fc.clearFlood(userID)
		return
	}
	if remaining > 10*time.Second {
		remaining = 10 * time.Second
	}
	time.Sleep(remaining)
	fc.clearFlood(userID)
}

func (fc *FloodControl) clearFlood(userID int64) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	until, ok := fc.floodUntil[userID]
	if ok && time.Now().After(until) {
		delete(fc.floodUntil, userID)
	}
}
