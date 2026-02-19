package monitor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/otaviocarvalho/tramuntana/internal/config"
	"github.com/otaviocarvalho/tramuntana/internal/state"
)

func TestWindowIDFromSessionKey(t *testing.T) {
	tests := []struct {
		key  string
		want string
	}{
		{"tramuntana:@5", "@5"},
		{"session:@12", "@12"},
		{"a:b:@3", "@3"},
		{"nowindow", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := windowIDFromSessionKey(tt.key)
			if got != tt.want {
				t.Errorf("windowIDFromSessionKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestMonitorNew(t *testing.T) {
	cfg := &config.Config{
		TramuntanaDir:       t.TempDir(),
		MonitorPollInterval: 2.0,
	}
	st := state.NewState()
	ms := state.NewMonitorState()

	m := New(cfg, st, ms, nil)
	if m == nil {
		t.Fatal("monitor should not be nil")
	}
	if m.pollInterval.Seconds() != 2.0 {
		t.Errorf("poll interval = %v, want 2s", m.pollInterval)
	}
}

func TestHasFileChanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")
	os.WriteFile(path, []byte(`{}`), 0o644)

	cfg := &config.Config{
		TramuntanaDir:       dir,
		MonitorPollInterval: 2.0,
	}
	m := New(cfg, state.NewState(), state.NewMonitorState(), nil)

	// First check should return true
	if !m.hasFileChanged(path) {
		t.Error("first check should detect change")
	}

	// Second check without modification should return false
	if m.hasFileChanged(path) {
		t.Error("unchanged file should not detect change")
	}

	// Modify file — sleep to ensure different mtime (filesystem may have 1s granularity)
	time.Sleep(10 * time.Millisecond)
	now := time.Now().Add(1 * time.Second)
	os.WriteFile(path, []byte(`{"updated":true}`), 0o644)
	os.Chtimes(path, now, now)
	if !m.hasFileChanged(path) {
		t.Error("modified file should detect change")
	}
}

func TestHasFileChanged_NonExistent(t *testing.T) {
	cfg := &config.Config{
		TramuntanaDir:       t.TempDir(),
		MonitorPollInterval: 2.0,
	}
	m := New(cfg, state.NewState(), state.NewMonitorState(), nil)

	if m.hasFileChanged("/nonexistent/file.jsonl") {
		t.Error("nonexistent file should not detect change")
	}
}

func TestProcessSession_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	// Write initial content
	os.WriteFile(path, []byte(`{"type":"assistant","message":{"content":"hello"}}`+"\n"), 0o644)

	cfg := &config.Config{
		TramuntanaDir:       dir,
		MonitorPollInterval: 2.0,
	}
	ms := state.NewMonitorState()

	// Set offset beyond file size to simulate /clear truncation
	ms.UpdateOffset("test:@1", "test-session", path, 99999)

	m := New(cfg, state.NewState(), ms, nil)

	// processSession should reset offset and not crash
	m.processSession("test:@1", "test-session", "@1", path)

	// Offset should be updated to actual content size
	tracked, ok := ms.GetTracked("test:@1")
	if !ok {
		t.Fatal("should have tracked session")
	}
	if tracked.LastByteOffset == 99999 {
		t.Error("offset should have been reset from 99999")
	}
}

func TestDetectChanges_RemovesStale(t *testing.T) {
	cfg := &config.Config{
		TramuntanaDir:       t.TempDir(),
		MonitorPollInterval: 2.0,
	}
	ms := state.NewMonitorState()
	ms.UpdateOffset("old:@1", "old", "/some/path", 100)

	m := New(cfg, state.NewState(), ms, nil)
	m.lastSessionMap = map[string]state.SessionMapEntry{
		"old:@1": {SessionID: "old"},
	}

	// New map without the old entry
	newMap := map[string]state.SessionMapEntry{}
	m.detectChanges(newMap)

	if _, ok := ms.GetTracked("old:@1"); ok {
		t.Error("stale session should be removed")
	}
}

func TestFindJSONLFile_SessionsIndex(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME not set")
	}

	// Create mock Claude projects dir
	dir := t.TempDir()
	projectDir := filepath.Join(dir, "test-project")
	os.MkdirAll(projectDir, 0o755)

	// Create sessions-index.json
	indexContent := `{"test-session-id": {"created": "2024-01-01"}}`
	os.WriteFile(filepath.Join(projectDir, "sessions-index.json"), []byte(indexContent), 0o644)

	// Create JSONL file
	os.WriteFile(filepath.Join(projectDir, "test-session-id.jsonl"), []byte(`{}`), 0o644)

	cfg := &config.Config{
		TramuntanaDir:       t.TempDir(),
		MonitorPollInterval: 2.0,
	}
	m := New(cfg, state.NewState(), state.NewMonitorState(), nil)

	path := m.searchSessionsIndex(
		filepath.Join(projectDir, "sessions-index.json"),
		"test-session-id",
		projectDir,
	)
	if path == "" {
		t.Error("should find JSONL file from sessions-index")
	}
}

func TestSearchJSONLFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "abc-123.jsonl"), []byte(`{}`), 0o644)
	os.WriteFile(filepath.Join(dir, "other.jsonl"), []byte(`{}`), 0o644)

	cfg := &config.Config{
		TramuntanaDir:       t.TempDir(),
		MonitorPollInterval: 2.0,
	}
	m := New(cfg, state.NewState(), state.NewMonitorState(), nil)

	path := m.searchJSONLFiles(dir, "abc-123")
	if path == "" {
		t.Error("should find JSONL file by name")
	}

	path = m.searchJSONLFiles(dir, "nonexistent")
	if path != "" {
		t.Error("should not find nonexistent session")
	}
}
