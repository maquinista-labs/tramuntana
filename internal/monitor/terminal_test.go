package monitor

import (
	"strings"
	"testing"
)

func TestStripPaneChrome(t *testing.T) {
	// Simulate Claude Code's terminal output with chrome
	lines := []string{
		"Some output line 1",
		"Some output line 2",
		"Some output line 3",
		strings.Repeat("─", 40),
		"> Enter a message...",
		"",
	}
	paneText := strings.Join(lines, "\n")

	got := StripPaneChrome(paneText)
	if strings.Contains(got, "Enter a message") {
		t.Error("should strip chrome below separator")
	}
	if !strings.Contains(got, "Some output line 3") {
		t.Error("should preserve content above separator")
	}
}

func TestStripPaneChrome_NoSeparator(t *testing.T) {
	paneText := "line1\nline2\nline3"
	got := StripPaneChrome(paneText)
	if got != paneText {
		t.Error("without separator, should return original text")
	}
}

func TestExtractStatusLine_WithSpinner(t *testing.T) {
	lines := []string{
		"Some content",
		"",
		"✻ Reading file.go",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	status, ok := ExtractStatusLine(paneText)
	if !ok {
		t.Fatal("should find status line")
	}
	if status != "Reading file.go" {
		t.Errorf("status = %q, want 'Reading file.go'", status)
	}
}

func TestExtractStatusLine_AllSpinnerChars(t *testing.T) {
	for _, spinner := range "·✻✽✶✳✢" {
		lines := []string{
			"content",
			string(spinner) + " Working...",
			strings.Repeat("─", 40),
			"> prompt",
		}
		paneText := strings.Join(lines, "\n")

		status, ok := ExtractStatusLine(paneText)
		if !ok {
			t.Errorf("should detect spinner %c", spinner)
			continue
		}
		if status != "Working..." {
			t.Errorf("spinner %c: status = %q, want 'Working...'", spinner, status)
		}
	}
}

func TestExtractStatusLine_NoSpinner(t *testing.T) {
	lines := []string{
		"Some content",
		"No spinner here",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	_, ok := ExtractStatusLine(paneText)
	if ok {
		t.Error("should not find status without spinner")
	}
}

func TestExtractStatusLine_NoSeparator(t *testing.T) {
	paneText := "✻ Working...\nno separator"
	_, ok := ExtractStatusLine(paneText)
	if ok {
		t.Error("should not find status without separator")
	}
}

func TestIsChromeSeparator(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{strings.Repeat("─", 40), true},
		{strings.Repeat("─", 20), true},
		{strings.Repeat("─", 19), false},
		{"some text", false},
		{"", false},
		{strings.Repeat("━", 25), true},
		{"  " + strings.Repeat("─", 25) + "  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.line[:min(len(tt.line), 20)], func(t *testing.T) {
			got := isChromeSeparator(tt.line)
			if got != tt.want {
				t.Errorf("isChromeSeparator(%q) = %v, want %v", tt.line[:min(len(tt.line), 20)], got, tt.want)
			}
		})
	}
}

func TestHasSpinnerChar(t *testing.T) {
	if !hasSpinnerChar("✻ Working") {
		t.Error("should detect ✻")
	}
	if !hasSpinnerChar("· Loading") {
		t.Error("should detect ·")
	}
	if hasSpinnerChar("No spinner here") {
		t.Error("should not detect spinner in plain text")
	}
}

func TestExtractAfterSpinner(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"✻ Working on task", "Working on task"},
		{"· Loading files", "Loading files"},
		{"✽   Multiple spaces", "Multiple spaces"},
		{"No spinner", ""},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractAfterSpinner(tt.line)
			if got != tt.want {
				t.Errorf("extractAfterSpinner(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestShortenSeparators(t *testing.T) {
	input := "line1\n" + strings.Repeat("─", 40) + "\nline2"
	got := ShortenSeparators(input)
	if !strings.Contains(got, "─────") {
		t.Error("should shorten separator")
	}
	if strings.Contains(got, strings.Repeat("─", 40)) {
		t.Error("should not have long separator")
	}
}

func TestIsInteractiveUI_ExitPlanMode(t *testing.T) {
	lines := []string{
		"Some content",
		"Would you like to proceed?",
		"Option 1",
		"Option 2",
		"ctrl-g to edit",
	}
	paneText := strings.Join(lines, "\n")
	if !IsInteractiveUI(paneText) {
		t.Error("should detect ExitPlanMode")
	}

	ui, ok := ExtractInteractiveContent(paneText)
	if !ok {
		t.Fatal("should extract content")
	}
	if ui.Name != "ExitPlanMode" {
		t.Errorf("name = %q, want ExitPlanMode", ui.Name)
	}
}

func TestIsInteractiveUI_PermissionPrompt(t *testing.T) {
	lines := []string{
		"Do you want to proceed?",
		"Allow this action?",
		"Esc to cancel",
	}
	paneText := strings.Join(lines, "\n")
	if !IsInteractiveUI(paneText) {
		t.Error("should detect PermissionPrompt")
	}

	ui, _ := ExtractInteractiveContent(paneText)
	if ui.Name != "PermissionPrompt" {
		t.Errorf("name = %q, want PermissionPrompt", ui.Name)
	}
}

func TestIsInteractiveUI_AskUserQuestion(t *testing.T) {
	lines := []string{
		"Which option?",
		"☐ Option A",
		"✔ Option B",
		"Enter to select",
	}
	paneText := strings.Join(lines, "\n")
	if !IsInteractiveUI(paneText) {
		t.Error("should detect AskUserQuestion")
	}
}

func TestIsInteractiveUI_None(t *testing.T) {
	paneText := "Just some regular output\nNothing special here\n"
	if IsInteractiveUI(paneText) {
		t.Error("should not detect interactive UI in plain text")
	}
}

func TestIsInteractiveUI_RestoreCheckpoint(t *testing.T) {
	lines := []string{
		"Restore the code to a previous state?",
		"Select checkpoint:",
		"Enter to continue",
	}
	paneText := strings.Join(lines, "\n")
	if !IsInteractiveUI(paneText) {
		t.Error("should detect RestoreCheckpoint")
	}
}

func TestExtractBashOutput_Found(t *testing.T) {
	lines := []string{
		"Some previous output",
		"! git status",
		"On branch main",
		"nothing to commit",
		"",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	got := ExtractBashOutput(paneText, "git status")
	if got == "" {
		t.Fatal("should find bash output")
	}
	if !strings.Contains(got, "! git status") {
		t.Error("should include command echo")
	}
	if !strings.Contains(got, "nothing to commit") {
		t.Error("should include output")
	}
}

func TestExtractBashOutput_NotFound(t *testing.T) {
	lines := []string{
		"Some regular output",
		"No bash command here",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	got := ExtractBashOutput(paneText, "git status")
	if got != "" {
		t.Errorf("should not find output, got %q", got)
	}
}

func TestExtractBashOutput_PrefixMatch(t *testing.T) {
	lines := []string{
		"! git status --porcelain --long --verbose...", // long command shown in pane
		"On branch main",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	// Should match on first 10 chars of command
	got := ExtractBashOutput(paneText, "git status --porcelain --long --verbose --show-stash")
	if got == "" {
		t.Fatal("should match on prefix")
	}
}

func TestExtractBashOutput_NoSpace(t *testing.T) {
	lines := []string{
		"!git status",
		"On branch main",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	got := ExtractBashOutput(paneText, "git status")
	if got == "" {
		t.Fatal("should match without space after !")
	}
}

func TestExtractBashOutput_StripsTrailingEmpty(t *testing.T) {
	lines := []string{
		"! ls",
		"file1.txt",
		"file2.txt",
		"",
		"",
		strings.Repeat("─", 40),
		"> prompt",
	}
	paneText := strings.Join(lines, "\n")

	got := ExtractBashOutput(paneText, "ls")
	if strings.HasSuffix(got, "\n") {
		t.Error("should strip trailing empty lines")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
