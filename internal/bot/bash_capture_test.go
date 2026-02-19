package bot

import (
	"testing"
)

func TestBashCaptureKey(t *testing.T) {
	key := bashCaptureKey(12345, 678)
	if key != "12345:678" {
		t.Errorf("got %q, want 12345:678", key)
	}
}

func TestCancelBashCapture_NoOp(t *testing.T) {
	// Should not panic when canceling a non-existent capture
	cancelBashCapture(999, 999)
}

func TestCancelBashCapture_CancelsExisting(t *testing.T) {
	cancelled := false
	key := bashCaptureKey(1, 2)

	bashCapturesMu.Lock()
	bashCaptures[key] = &bashCapture{
		cancel: func() { cancelled = true },
	}
	bashCapturesMu.Unlock()

	cancelBashCapture(1, 2)
	if !cancelled {
		t.Error("should have called cancel")
	}

	// Should be removed from map
	bashCapturesMu.Lock()
	_, exists := bashCaptures[key]
	bashCapturesMu.Unlock()
	if exists {
		t.Error("should have been removed from map")
	}
}

func TestBashCaptureConstants(t *testing.T) {
	if bashCaptureMaxPolls != 30 {
		t.Errorf("max polls = %d, want 30", bashCaptureMaxPolls)
	}
	if bashCaptureMaxChars != 3800 {
		t.Errorf("max chars = %d, want 3800", bashCaptureMaxChars)
	}
}
