package bot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFormatHistoryEntry_Text(t *testing.T) {
	entry := historyEntry{Role: "assistant", ContentType: "text", Text: "Hello world"}
	got := formatHistoryEntry(entry)
	if got != "> Hello world" {
		t.Errorf("got %q", got)
	}
}

func TestFormatHistoryEntry_UserText(t *testing.T) {
	entry := historyEntry{Role: "user", ContentType: "text", Text: "User message"}
	got := formatHistoryEntry(entry)
	if got != "You: User message" {
		t.Errorf("got %q", got)
	}
}

func TestFormatHistoryEntry_ToolUse(t *testing.T) {
	entry := historyEntry{ContentType: "tool_use", Text: "**Read**(file.go)"}
	got := formatHistoryEntry(entry)
	if got != "Tool: **Read**(file.go)" {
		t.Errorf("got %q", got)
	}
}

func TestFormatHistoryEntry_ToolResult(t *testing.T) {
	entry := historyEntry{ContentType: "tool_result", ToolName: "Read", Text: "line1\nline2\nline3"}
	got := formatHistoryEntry(entry)
	if got != "Result [Read]: 3 lines" {
		t.Errorf("got %q", got)
	}
}

func TestFormatHistoryEntry_ToolResultError(t *testing.T) {
	entry := historyEntry{ContentType: "tool_result", ToolName: "Bash", Text: "error", IsError: true}
	got := formatHistoryEntry(entry)
	if got != "Result [Bash]: ERROR (1 lines)" {
		t.Errorf("got %q", got)
	}
}

func TestFormatHistoryEntry_Thinking(t *testing.T) {
	entry := historyEntry{ContentType: "thinking", Text: "Let me consider..."}
	got := formatHistoryEntry(entry)
	if got != "Thinking: Let me consider..." {
		t.Errorf("got %q", got)
	}
}

func TestTruncateText(t *testing.T) {
	tests := []struct {
		text   string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"this is a longer string", 10, "this is a ..."},
		{"line1\nline2", 100, "line1"},
		{"", 10, ""},
	}
	for _, tt := range tests {
		got := truncateText(tt.text, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateText(%q, %d) = %q, want %q", tt.text, tt.maxLen, got, tt.want)
		}
	}
}

func TestBuildHistoryKeyboard_SinglePage(t *testing.T) {
	kb := buildHistoryKeyboard("@1", 0, 1)
	if kb != nil {
		t.Error("should return nil for single page")
	}
}

func TestBuildHistoryKeyboard_FirstPage(t *testing.T) {
	kb := buildHistoryKeyboard("@1", 0, 5)
	if kb == nil {
		t.Fatal("should return keyboard")
	}
	row := kb.InlineKeyboard[0]
	// Should have page counter + Newer (no Older on first page)
	if len(row) != 2 {
		t.Fatalf("expected 2 buttons on first page, got %d", len(row))
	}
	if row[0].Text != "1/5" {
		t.Errorf("first button = %q, want page counter", row[0].Text)
	}
	if row[1].Text != "Newer" {
		t.Errorf("second button = %q, want Newer", row[1].Text)
	}
}

func TestBuildHistoryKeyboard_LastPage(t *testing.T) {
	kb := buildHistoryKeyboard("@1", 4, 5)
	if kb == nil {
		t.Fatal("should return keyboard")
	}
	row := kb.InlineKeyboard[0]
	// Should have Older + page counter (no Newer on last page)
	if len(row) != 2 {
		t.Fatalf("expected 2 buttons on last page, got %d", len(row))
	}
	if row[0].Text != "Older" {
		t.Errorf("first button = %q, want Older", row[0].Text)
	}
}

func TestBuildHistoryKeyboard_MiddlePage(t *testing.T) {
	kb := buildHistoryKeyboard("@1", 2, 5)
	if kb == nil {
		t.Fatal("should return keyboard")
	}
	row := kb.InlineKeyboard[0]
	// Should have Older + page counter + Newer
	if len(row) != 3 {
		t.Fatalf("expected 3 buttons on middle page, got %d", len(row))
	}
}

func TestParseHistCallbackData(t *testing.T) {
	tests := []struct {
		data     string
		page     int
		windowID string
		ok       bool
	}{
		{"hist_0:@1", 0, "@1", true},
		{"hist_5:@10", 5, "@10", true},
		{"hist_nope:@1", 0, "", false},
		{"ss_up:@1", 0, "", false},
		{"hist_0", 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.data, func(t *testing.T) {
			page, windowID, ok := parseHistCallbackData(tt.data)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if ok {
				if page != tt.page {
					t.Errorf("page = %d, want %d", page, tt.page)
				}
				if windowID != tt.windowID {
					t.Errorf("windowID = %q, want %q", windowID, tt.windowID)
				}
			}
		})
	}
}

func TestFormatHistCallback(t *testing.T) {
	data := formatHistCallback(3, "@1")
	if data != "hist_3:@1" {
		t.Errorf("got %q, want hist_3:@1", data)
	}
}

func TestFormatHistoryPage(t *testing.T) {
	entries := make([]historyEntry, 25)
	for i := range entries {
		entries[i] = historyEntry{Role: "assistant", ContentType: "text", Text: "Message " + string(rune('A'+i))}
	}

	text := formatHistoryPage(entries, 0, "@1")
	if text == "" {
		t.Error("should produce non-empty text")
	}
	// First page should show page 1/3
	if !contains(text, "Page 1/3") {
		t.Errorf("should show page 1/3, got: %s", text)
	}
	// Should have 10 entries on first page
	count := 0
	for _, e := range entries[:10] {
		if contains(text, e.Text) {
			count++
		}
	}
	if count != 10 {
		t.Errorf("expected 10 entries on first page, found %d", count)
	}
}

func TestReadAllEntries(t *testing.T) {
	// Create a temp JSONL file with a simple entry
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "test.jsonl")
	jsonl := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello"}]}}` + "\n"
	os.WriteFile(jsonlPath, []byte(jsonl), 0644)

	entries := readAllEntries(jsonlPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ContentType != "text" {
		t.Errorf("content type = %q, want text", entries[0].ContentType)
	}
	if entries[0].Text != "Hello" {
		t.Errorf("text = %q, want Hello", entries[0].Text)
	}
}

func TestReadAllEntries_Empty(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(jsonlPath, []byte(""), 0644)

	entries := readAllEntries(jsonlPath)
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestReadAllEntries_NonExistent(t *testing.T) {
	entries := readAllEntries("/nonexistent/file.jsonl")
	if entries != nil {
		t.Errorf("expected nil for nonexistent file, got %v", entries)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsStr(s, substr)))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
