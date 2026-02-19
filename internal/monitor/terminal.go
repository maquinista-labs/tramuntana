package monitor

import (
	"strings"
	"unicode/utf8"
)

// Spinner characters used by Claude Code's status line.
const spinnerChars = "·✻✽✶✳✢"

// StripPaneChrome removes Claude Code's bottom chrome (separator, prompt, status bar)
// from captured pane text. Returns the text above the separator.
func StripPaneChrome(paneText string) string {
	lines := strings.Split(paneText, "\n")
	sepIdx := findChromeSeparator(lines)
	if sepIdx < 0 {
		return paneText
	}
	return strings.Join(lines[:sepIdx], "\n")
}

// ExtractStatusLine detects Claude's spinner/status from the terminal output.
// Returns the status text and whether a status was found.
func ExtractStatusLine(paneText string) (string, bool) {
	lines := strings.Split(paneText, "\n")
	sepIdx := findChromeSeparator(lines)
	if sepIdx < 0 {
		return "", false
	}

	// Look at lines above the separator for spinner characters
	// Check up to 3 lines above
	searchStart := sepIdx - 3
	if searchStart < 0 {
		searchStart = 0
	}

	for i := sepIdx - 1; i >= searchStart; i-- {
		line := strings.TrimSpace(lines[i])
		if hasSpinnerChar(line) {
			// Extract the text after the spinner character
			statusText := extractAfterSpinner(line)
			if statusText != "" {
				return statusText, true
			}
		}
	}

	return "", false
}

// findChromeSeparator finds the line index of the chrome separator
// (a line of ─ chars, ≥20 wide) in the last 10 lines.
func findChromeSeparator(lines []string) int {
	start := len(lines) - 10
	if start < 0 {
		start = 0
	}

	for i := len(lines) - 1; i >= start; i-- {
		if isChromeSeparator(lines[i]) {
			return i
		}
	}
	return -1
}

// isChromeSeparator checks if a line is a chrome separator (≥20 ─ chars).
func isChromeSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}

	dashCount := 0
	for _, r := range trimmed {
		if r == '─' || r == '━' {
			dashCount++
		}
	}

	return dashCount >= 20
}

// hasSpinnerChar checks if a line contains a spinner character.
func hasSpinnerChar(line string) bool {
	for _, r := range line {
		if strings.ContainsRune(spinnerChars, r) {
			return true
		}
	}
	return false
}

// extractAfterSpinner extracts the text after the first spinner character.
func extractAfterSpinner(line string) string {
	for i, r := range line {
		if strings.ContainsRune(spinnerChars, r) {
			rest := strings.TrimSpace(line[i+utf8.RuneLen(r):])
			return rest
		}
	}
	return ""
}

// ShortenSeparators replaces long ─ lines with a shorter version for display.
func ShortenSeparators(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if isChromeSeparator(line) {
			lines[i] = "─────"
		}
	}
	return strings.Join(lines, "\n")
}
