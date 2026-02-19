package bot

import (
	"testing"
)

func TestBuildScreenshotKeyboard(t *testing.T) {
	kb := buildScreenshotKeyboard("@1")
	if len(kb.InlineKeyboard) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(kb.InlineKeyboard))
	}
	// Row 1: arrow keys (4 buttons)
	if len(kb.InlineKeyboard[0]) != 4 {
		t.Errorf("row 1 should have 4 buttons, got %d", len(kb.InlineKeyboard[0]))
	}
	// Row 2: Space, Tab, Esc, Enter (4 buttons)
	if len(kb.InlineKeyboard[1]) != 4 {
		t.Errorf("row 2 should have 4 buttons, got %d", len(kb.InlineKeyboard[1]))
	}
	// Row 3: Refresh (1 button)
	if len(kb.InlineKeyboard[2]) != 1 {
		t.Errorf("row 3 should have 1 button, got %d", len(kb.InlineKeyboard[2]))
	}
}

func TestBuildScreenshotKeyboard_CallbackData(t *testing.T) {
	kb := buildScreenshotKeyboard("@5")
	// Check that all callback data starts with "ss_"
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				t.Error("button should have callback data")
				continue
			}
			data := *btn.CallbackData
			if len(data) < 4 || data[:3] != "ss_" {
				t.Errorf("callback data %q should start with ss_", data)
			}
			if len(data) > 64 {
				t.Errorf("callback data %q exceeds 64 bytes", data)
			}
		}
	}
}

func TestParseSSCallbackData(t *testing.T) {
	tests := []struct {
		data     string
		action   string
		windowID string
		ok       bool
	}{
		{"ss_up:@1", "up", "@1", true},
		{"ss_down:@5", "down", "@5", true},
		{"ss_refresh:@10", "refresh", "@10", true},
		{"ss_enter:@2", "enter", "@2", true},
		{"ss_nocolon", "", "", false},
		{"nav_up:@1", "", "", false},
		{"", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.data, func(t *testing.T) {
			action, windowID, ok := parseSSCallbackData(tt.data)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if action != tt.action {
				t.Errorf("action = %q, want %q", action, tt.action)
			}
			if windowID != tt.windowID {
				t.Errorf("windowID = %q, want %q", windowID, tt.windowID)
			}
		})
	}
}

func TestFormatSSCallback(t *testing.T) {
	data := formatSSCallback("up", "@1")
	if data != "ss_up:@1" {
		t.Errorf("got %q, want ss_up:@1", data)
	}

	// Test truncation at 64 bytes
	longWindowID := "@" + string(make([]byte, 100))
	data = formatSSCallback("refresh", longWindowID)
	if len(data) > 64 {
		t.Errorf("callback data should be at most 64 bytes, got %d", len(data))
	}
}

func TestScreenshotCallbackActions(t *testing.T) {
	actions := screenshotCallbackActions()
	expected := map[string]bool{
		"up": true, "down": true, "left": true, "right": true,
		"space": true, "tab": true, "esc": true, "enter": true,
		"refresh": true,
	}
	if len(actions) != len(expected) {
		t.Fatalf("expected %d actions, got %d", len(expected), len(actions))
	}
	for _, a := range actions {
		if !expected[a] {
			t.Errorf("unexpected action: %s", a)
		}
	}
}

func TestSSKeyMap(t *testing.T) {
	// All key actions (except refresh) should map to tmux key names
	for _, action := range screenshotCallbackActions() {
		if action == "refresh" {
			continue
		}
		tmuxKey, ok := ssKeyMap[action]
		if !ok {
			t.Errorf("action %q has no tmux key mapping", action)
		}
		if tmuxKey == "" {
			t.Errorf("action %q maps to empty key", action)
		}
	}
}

func TestScreenshotKey(t *testing.T) {
	key := screenshotKey(12345, 678)
	if key != "12345:678" {
		t.Errorf("got %q, want 12345:678", key)
	}
}
