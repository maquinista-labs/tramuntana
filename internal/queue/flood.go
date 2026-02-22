package queue

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var retryAfterRe = regexp.MustCompile(`retry after (\d+)`)

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
	if !strings.Contains(errStr, "Too Many Requests") && !strings.Contains(errStr, "429") {
		return
	}

	// Parse actual retry-after value, default to 30s
	wait := 30 * time.Second
	if m := retryAfterRe.FindStringSubmatch(errStr); len(m) == 2 {
		if secs, err := strconv.Atoi(m[1]); err == nil && secs > 0 {
			wait = time.Duration(secs)*time.Second + time.Second // +1s margin
		}
	}

	fc.mu.Lock()
	newUntil := time.Now().Add(wait)
	// Only extend, never shorten an existing ban
	if existing, ok := fc.floodUntil[chatID]; !ok || newUntil.After(existing) {
		fc.floodUntil[chatID] = newUntil
		fmt.Printf("Flood control: chat %d rate-limited for %v\n", chatID, wait)
	}
	fc.mu.Unlock()
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

// WaitIfFlooded blocks until the flood ban expires.
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
