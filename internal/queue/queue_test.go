package queue

import (
	"testing"
	"time"
)

func TestFloodControl_NotFlooded(t *testing.T) {
	fc := NewFloodControl()
	if fc.IsFlooded(100) {
		t.Error("should not be flooded initially")
	}
}

func TestFloodControl_SetFlood(t *testing.T) {
	fc := NewFloodControl()
	fc.mu.Lock()
	fc.floodUntil[100] = time.Now().Add(1 * time.Second)
	fc.mu.Unlock()

	if !fc.IsFlooded(100) {
		t.Error("should be flooded")
	}

	// Wait for expiry
	time.Sleep(1100 * time.Millisecond)
	if fc.IsFlooded(100) {
		t.Error("should not be flooded after expiry")
	}
}

func TestFloodControl_HandleError429(t *testing.T) {
	fc := NewFloodControl()
	fc.HandleError(100, &mockError{"Too Many Requests: retry after 5"})

	if !fc.IsFlooded(100) {
		t.Error("should be flooded after 429")
	}
}

func TestFloodControl_HandleNon429(t *testing.T) {
	fc := NewFloodControl()
	fc.HandleError(100, &mockError{"Bad Request"})

	if fc.IsFlooded(100) {
		t.Error("should not be flooded for non-429 error")
	}
}

func TestFloodControl_HandleNil(t *testing.T) {
	fc := NewFloodControl()
	fc.HandleError(100, nil) // should not panic
}

func TestMessageTaskTypes(t *testing.T) {
	types := []string{"content", "tool_use", "tool_result", "status_update", "status_clear"}
	for _, ct := range types {
		task := MessageTask{ContentType: ct}
		if task.ContentType != ct {
			t.Errorf("got %q, want %q", task.ContentType, ct)
		}
	}
}

func TestUserThread(t *testing.T) {
	ut1 := userThread{100, 42}
	ut2 := userThread{100, 42}
	ut3 := userThread{100, 43}

	if ut1 != ut2 {
		t.Error("same user+thread should be equal")
	}
	if ut1 == ut3 {
		t.Error("different threads should not be equal")
	}
}

func TestMergeLimit(t *testing.T) {
	// Verify maxMergeLen constant
	if maxMergeLen != 3800 {
		t.Errorf("maxMergeLen = %d, want 3800", maxMergeLen)
	}
}

func TestStatusInfo(t *testing.T) {
	info := StatusInfo{
		MessageID: 123,
		WindowID:  "@5",
		Text:      "Working...",
	}
	if info.MessageID != 123 {
		t.Error("wrong message ID")
	}
	if info.WindowID != "@5" {
		t.Error("wrong window ID")
	}
}

type mockError struct {
	msg string
}

func (e *mockError) Error() string {
	return e.msg
}
