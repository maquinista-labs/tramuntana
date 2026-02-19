package monitor

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/otaviocarvalho/tramuntana/internal/config"
	"github.com/otaviocarvalho/tramuntana/internal/queue"
	"github.com/otaviocarvalho/tramuntana/internal/render"
	"github.com/otaviocarvalho/tramuntana/internal/state"
)

// Monitor polls Claude Code JSONL transcript files and routes entries to the message queue.
type Monitor struct {
	config         *config.Config
	state          *state.State
	monitorState   *state.MonitorState
	queue          *queue.Queue
	pendingTools   map[string]PendingTool
	fileMtimes     map[string]time.Time
	lastSessionMap map[string]state.SessionMapEntry
	pollInterval   time.Duration
}

// New creates a new Monitor.
func New(cfg *config.Config, st *state.State, ms *state.MonitorState, q *queue.Queue) *Monitor {
	return &Monitor{
		config:         cfg,
		state:          st,
		monitorState:   ms,
		queue:          q,
		pendingTools:   make(map[string]PendingTool),
		fileMtimes:     make(map[string]time.Time),
		lastSessionMap: make(map[string]state.SessionMapEntry),
		pollInterval:   time.Duration(cfg.MonitorPollInterval * float64(time.Second)),
	}
}

// Run starts the monitor poll loop. Blocks until ctx is cancelled.
func (m *Monitor) Run(ctx context.Context) {
	log.Println("Session monitor starting...")
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.monitorState.ForceSave(filepath.Join(m.config.TramuntanaDir, "monitor_state.json"))
			log.Println("Session monitor stopped.")
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) poll() {
	// Load session_map.json
	sessionMapPath := filepath.Join(m.config.TramuntanaDir, "session_map.json")
	sm, err := state.LoadSessionMap(sessionMapPath)
	if err != nil {
		return
	}

	// Detect changes
	m.detectChanges(sm)

	// Process each active session
	for key, entry := range sm {
		windowID := windowIDFromSessionKey(key)
		if windowID == "" {
			continue
		}

		// Find the JSONL file for this session
		jsonlPath := m.findJSONLFile(entry.SessionID, entry.CWD)
		if jsonlPath == "" {
			continue
		}

		// Check mtime
		if !m.hasFileChanged(jsonlPath) {
			continue
		}

		// Read new content
		m.processSession(key, entry.SessionID, windowID, jsonlPath)
	}

	m.lastSessionMap = sm

	// Periodically save state
	monitorStatePath := filepath.Join(m.config.TramuntanaDir, "monitor_state.json")
	m.monitorState.SaveIfDirty(monitorStatePath)
}

func (m *Monitor) detectChanges(newMap map[string]state.SessionMapEntry) {
	// Clean up stale sessions
	for key := range m.lastSessionMap {
		if _, ok := newMap[key]; !ok {
			m.monitorState.RemoveSession(key)
			delete(m.fileMtimes, key)
		}
	}
}

func (m *Monitor) hasFileChanged(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	mtime := info.ModTime()
	lastMtime, ok := m.fileMtimes[path]
	if ok && mtime.Equal(lastMtime) {
		return false
	}

	m.fileMtimes[path] = mtime
	return true
}

func (m *Monitor) processSession(sessionKey, sessionID, windowID, jsonlPath string) {
	// Get current offset
	tracked, hasTracked := m.monitorState.GetTracked(sessionKey)
	var offset int64
	if hasTracked {
		offset = tracked.LastByteOffset
	}

	// Check file size (detect truncation from /clear)
	info, err := os.Stat(jsonlPath)
	if err != nil {
		return
	}
	if offset > info.Size() {
		offset = 0 // file was truncated
	}

	// Open and read new content
	f, err := os.Open(jsonlPath)
	if err != nil {
		return
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return
		}
	}

	var entries []*Entry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large lines
	var bytesRead int64

	for scanner.Scan() {
		line := scanner.Bytes()
		bytesRead += int64(len(line)) + 1 // +1 for newline

		entry, err := ParseLine(line)
		if err != nil {
			log.Printf("JSONL parse error at offset %d: %v", offset+bytesRead, err)
			continue
		}
		if entry != nil {
			entries = append(entries, entry)
		}
	}

	if len(entries) == 0 {
		// Update offset even if no entries (skip empty lines)
		if bytesRead > 0 {
			newOffset := offset + bytesRead
			m.monitorState.UpdateOffset(sessionKey, sessionID, jsonlPath, newOffset)
		}
		return
	}

	// Parse entries with tool pairing
	parsed := ParseEntries(entries, m.pendingTools)

	// Route to users
	users := m.state.FindUsersForWindow(windowID)
	for _, ut := range users {
		chatID, ok := m.state.GetGroupChatID(ut.UserID, ut.ThreadID)
		if !ok {
			continue
		}
		threadID, _ := strconv.Atoi(ut.ThreadID)
		userID, _ := strconv.ParseInt(ut.UserID, 10, 64)

		for _, pe := range parsed {
			m.enqueueEntry(userID, threadID, chatID, windowID, pe)
		}
	}

	// Update offset
	newOffset := offset + bytesRead
	m.monitorState.UpdateOffset(sessionKey, sessionID, jsonlPath, newOffset)
}

func (m *Monitor) enqueueEntry(userID int64, threadID int, chatID int64, windowID string, pe ParsedEntry) {
	var text string
	var contentType string

	switch pe.ContentType {
	case "text":
		text = render.FormatText(pe.Text)
		contentType = "content"
	case "tool_use":
		text = render.FormatToolUse(pe.ToolName, "")
		if pe.Text != "" {
			text = pe.Text // use the pre-formatted summary
		}
		contentType = "tool_use"
	case "tool_result":
		text = render.FormatToolResult(pe.ToolName, pe.Text, pe.IsError)
		contentType = "tool_result"
	case "thinking":
		text = render.FormatThinking(pe.Text)
		contentType = "content"
	default:
		return
	}

	if text == "" {
		return
	}

	m.queue.Enqueue(queue.MessageTask{
		UserID:      userID,
		ThreadID:    threadID,
		ChatID:      chatID,
		Parts:       []string{text},
		ContentType: contentType,
		ToolUseID:   pe.ToolUseID,
		WindowID:    windowID,
	})
}

// findJSONLFile locates the JSONL transcript file for a session.
func (m *Monitor) findJSONLFile(sessionID, cwd string) string {
	// First: check monitor state for cached path
	for _, key := range m.monitorState.AllKeys() {
		tracked, ok := m.monitorState.GetTracked(key)
		if ok && tracked.SessionID == sessionID && tracked.FilePath != "" {
			if _, err := os.Stat(tracked.FilePath); err == nil {
				return tracked.FilePath
			}
		}
	}

	// Second: scan ~/.claude/projects/ for matching session
	claudeDir := filepath.Join(os.Getenv("HOME"), ".claude", "projects")
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		return ""
	}

	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}

		projectDir := filepath.Join(claudeDir, dir.Name())

		// Check sessions-index.json
		indexPath := filepath.Join(projectDir, "sessions-index.json")
		if path := m.searchSessionsIndex(indexPath, sessionID, projectDir); path != "" {
			return path
		}

		// Fallback: glob for JSONL files
		if path := m.searchJSONLFiles(projectDir, sessionID); path != "" {
			return path
		}
	}

	return ""
}

func (m *Monitor) searchSessionsIndex(indexPath, sessionID, projectDir string) string {
	data, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}

	var index map[string]json.RawMessage
	if err := json.Unmarshal(data, &index); err != nil {
		return ""
	}

	for id := range index {
		if id == sessionID {
			jsonlPath := filepath.Join(projectDir, id+".jsonl")
			if _, err := os.Stat(jsonlPath); err == nil {
				return jsonlPath
			}
		}
	}
	return ""
}

func (m *Monitor) searchJSONLFiles(projectDir, sessionID string) string {
	matches, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil {
		return ""
	}

	for _, match := range matches {
		base := filepath.Base(match)
		if strings.TrimSuffix(base, ".jsonl") == sessionID {
			return match
		}
	}
	return ""
}

// windowIDFromSessionKey extracts window ID from session key ("sessionName:@N" → "@N").
func windowIDFromSessionKey(key string) string {
	idx := strings.LastIndex(key, ":")
	if idx < 0 {
		return ""
	}
	return key[idx+1:]
}
