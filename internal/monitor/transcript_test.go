package monitor

import (
	"testing"
)

func TestParseLine_AssistantText(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "assistant" {
		t.Errorf("type = %q, want assistant", entry.Type)
	}
	if len(entry.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(entry.Blocks))
	}
	if entry.Blocks[0].Type != "text" {
		t.Errorf("block type = %q, want text", entry.Blocks[0].Type)
	}
	if entry.Blocks[0].Text != "Hello world" {
		t.Errorf("text = %q, want Hello world", entry.Blocks[0].Text)
	}
}

func TestParseLine_UserText(t *testing.T) {
	line := []byte(`{"type":"user","message":{"content":"fix the bug"}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "user" {
		t.Errorf("type = %q, want user", entry.Type)
	}
	if len(entry.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(entry.Blocks))
	}
	if entry.Blocks[0].Text != "fix the bug" {
		t.Errorf("text = %q, want 'fix the bug'", entry.Blocks[0].Text)
	}
}

func TestParseLine_ToolUse(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_123","name":"Read","input":{"file_path":"/tmp/test.go"}}]}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(entry.Blocks))
	}
	block := entry.Blocks[0]
	if block.Type != "tool_use" {
		t.Errorf("type = %q, want tool_use", block.Type)
	}
	if block.ToolName != "Read" {
		t.Errorf("tool name = %q, want Read", block.ToolName)
	}
	if block.ToolUseID != "tu_123" {
		t.Errorf("tool use id = %q, want tu_123", block.ToolUseID)
	}
	if block.ToolInput != "/tmp/test.go" {
		t.Errorf("tool input = %q, want /tmp/test.go", block.ToolInput)
	}
}

func TestParseLine_ToolResult(t *testing.T) {
	line := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu_123","content":"file contents here","is_error":false}]}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(entry.Blocks))
	}
	block := entry.Blocks[0]
	if block.Type != "tool_result" {
		t.Errorf("type = %q, want tool_result", block.Type)
	}
	if block.ToolUseID != "tu_123" {
		t.Errorf("tool use id = %q, want tu_123", block.ToolUseID)
	}
	if block.Content != "file contents here" {
		t.Errorf("content = %q, want 'file contents here'", block.Content)
	}
}

func TestParseLine_ToolResultError(t *testing.T) {
	line := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu_err","content":"command failed","is_error":true}]}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	block := entry.Blocks[0]
	if !block.IsError {
		t.Error("expected IsError to be true")
	}
}

func TestParseLine_Thinking(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"Let me think about this..."}]}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(entry.Blocks))
	}
	block := entry.Blocks[0]
	if block.Type != "thinking" {
		t.Errorf("type = %q, want thinking", block.Type)
	}
	if block.Text != "Let me think about this..." {
		t.Errorf("text = %q", block.Text)
	}
}

func TestParseLine_Summary(t *testing.T) {
	line := []byte(`{"type":"summary","message":{"content":"summary text"}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Type != "summary" {
		t.Errorf("type = %q, want summary", entry.Type)
	}
}

func TestParseLine_UnknownType(t *testing.T) {
	line := []byte(`{"type":"system","message":{}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if entry != nil {
		t.Error("unknown type should return nil")
	}
}

func TestParseLine_InvalidJSON(t *testing.T) {
	_, err := ParseLine([]byte(`not json`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseLine_MultipleBlocks(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"Looking at the file"},{"type":"tool_use","id":"tu_1","name":"Read","input":{"file_path":"main.go"}}]}}`)
	entry, err := ParseLine(line)
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(entry.Blocks))
	}
	if entry.Blocks[0].Type != "text" {
		t.Errorf("block 0 type = %q, want text", entry.Blocks[0].Type)
	}
	if entry.Blocks[1].Type != "tool_use" {
		t.Errorf("block 1 type = %q, want tool_use", entry.Blocks[1].Type)
	}
}

func TestExtractToolInput_AllTools(t *testing.T) {
	tests := []struct {
		tool  string
		input string
		want  string
	}{
		{"Read", `{"file_path":"/tmp/file.go"}`, "/tmp/file.go"},
		{"Write", `{"file_path":"/tmp/out.txt"}`, "/tmp/out.txt"},
		{"Edit", `{"file_path":"/tmp/edit.go"}`, "/tmp/edit.go"},
		{"Bash", `{"command":"git status"}`, "git status"},
		{"Grep", `{"pattern":"TODO"}`, "TODO"},
		{"Glob", `{"pattern":"**/*.go"}`, "**/*.go"},
		{"Task", `{"description":"search for code"}`, "search for code"},
		{"WebFetch", `{"url":"https://example.com"}`, "https://example.com"},
		{"WebSearch", `{"query":"golang error handling"}`, "golang error handling"},
		{"AskUserQuestion", `{"questions":[]}`, "interactive"},
		{"ExitPlanMode", `{}`, "plan"},
		{"Skill", `{"skill":"commit"}`, "commit"},
		{"Unknown", `{"foo":"bar"}`, ""},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			got := extractToolInput(tt.tool, []byte(tt.input))
			if got != tt.want {
				t.Errorf("extractToolInput(%q) = %q, want %q", tt.tool, got, tt.want)
			}
		})
	}
}

func TestExtractToolInput_BashTruncation(t *testing.T) {
	longCmd := ""
	for i := 0; i < 120; i++ {
		longCmd += "x"
	}
	input := `{"command":"` + longCmd + `"}`
	got := extractToolInput("Bash", []byte(input))
	if len(got) > 103 { // 100 + "..."
		t.Errorf("bash command not truncated: %d chars", len(got))
	}
}

func TestToolPairing_SameBatch(t *testing.T) {
	pending := make(map[string]PendingTool)

	// Parse assistant entry with tool_use
	assistantLine := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_abc","name":"Read","input":{"file_path":"main.go"}}]}}`)
	entry1, _ := ParseLine(assistantLine)

	// Parse user entry with tool_result
	userLine := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu_abc","content":"package main\n"}]}}`)
	entry2, _ := ParseLine(userLine)

	// Same-batch: tool_use is suppressed, only tool_result emitted
	results := ParseEntries([]*Entry{entry1, entry2}, pending)

	if len(results) != 1 {
		t.Fatalf("expected 1 result (combined), got %d", len(results))
	}

	// Combined tool_result with tool name from pairing
	if results[0].ContentType != "tool_result" {
		t.Errorf("result 0 type = %q, want tool_result", results[0].ContentType)
	}
	if results[0].ToolName != "Read" {
		t.Errorf("result 0 tool = %q, want Read", results[0].ToolName)
	}

	// Pending should be empty
	if len(pending) != 0 {
		t.Errorf("pending should be empty, got %d", len(pending))
	}
}

func TestToolPairing_CrossCycle(t *testing.T) {
	pending := make(map[string]PendingTool)

	// Cycle 1: tool_use only
	assistantLine := []byte(`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu_cross","name":"Bash","input":{"command":"ls"}}]}}`)
	entry1, _ := ParseLine(assistantLine)
	ParseEntries([]*Entry{entry1}, pending)

	// Pending should have one entry
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}

	// Cycle 2: tool_result
	userLine := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu_cross","content":"file1\nfile2\n"}]}}`)
	entry2, _ := ParseLine(userLine)
	results := ParseEntries([]*Entry{entry2}, pending)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ToolName != "Bash" {
		t.Errorf("tool = %q, want Bash", results[0].ToolName)
	}
	if len(pending) != 0 {
		t.Errorf("pending should be empty after pairing")
	}
}

func TestFormatToolUseSummary(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"Read", "main.go", "**Read**(main.go)"},
		{"Bash", "ls -la", "**Bash**(ls -la)"},
		{"Task", "", "**Task**()"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatToolUseSummary(tt.name, tt.input)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanText_StripsTags(t *testing.T) {
	text := "Hello <system-reminder>secret</system-reminder> world"
	got := cleanText(text)
	if got != "Hello  world" {
		t.Errorf("cleanText = %q, want 'Hello  world'", got)
	}
}

func TestCleanText_PreservesNormal(t *testing.T) {
	text := "Hello world"
	got := cleanText(text)
	if got != "Hello world" {
		t.Errorf("cleanText = %q, want 'Hello world'", got)
	}
}

func TestParseEntries_TextAndThinking(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"deep thought"},{"type":"text","text":"The answer is 42"}]}}`)
	entry, _ := ParseLine(line)

	pending := make(map[string]PendingTool)
	results := ParseEntries([]*Entry{entry}, pending)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].ContentType != "thinking" {
		t.Errorf("result 0 = %q, want thinking", results[0].ContentType)
	}
	if results[1].ContentType != "text" {
		t.Errorf("result 1 = %q, want text", results[1].ContentType)
	}
}

func TestToolResultContent_Array(t *testing.T) {
	line := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"tu_arr","content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}]}}`)
	entry, _ := ParseLine(line)
	if len(entry.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(entry.Blocks))
	}
	if entry.Blocks[0].Content != "line1\nline2" {
		t.Errorf("content = %q, want 'line1\\nline2'", entry.Blocks[0].Content)
	}
}
