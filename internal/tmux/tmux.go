package tmux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Window represents a tmux window.
type Window struct {
	ID   string // e.g. "@12"
	Name string
	CWD  string
}

// SessionExists checks if a tmux session exists.
func SessionExists(name string) bool {
	return exec.Command("tmux", "has-session", "-t", name).Run() == nil
}

// EnsureSession creates a tmux session if it doesn't exist.
func EnsureSession(name string) error {
	if SessionExists(name) {
		return nil
	}
	cmd := exec.Command("tmux", "new-session", "-d", "-s", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating session %s: %s: %w", name, string(out), err)
	}
	return nil
}

// ListWindows returns all windows in a session.
func ListWindows(session string) ([]Window, error) {
	cmd := exec.Command("tmux", "list-windows", "-t", session,
		"-F", "#{window_id}\t#{window_name}\t#{pane_current_path}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing windows in %s: %w", session, err)
	}

	var windows []Window
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		windows = append(windows, Window{
			ID:   parts[0],
			Name: parts[1],
			CWD:  parts[2],
		})
	}
	return windows, nil
}

// NewWindow creates a new window, sets env vars, and starts the Claude command.
// Returns the window ID.
func NewWindow(session, name, dir, claudeCmd string, env map[string]string) (string, error) {
	args := []string{"new-window", "-t", session, "-n", name, "-c", dir, "-P", "-F", "#{window_id}"}
	cmd := exec.Command("tmux", args...)
	cmdEnv := os.Environ()
	for k, v := range env {
		cmdEnv = append(cmdEnv, k+"="+v)
	}
	cmd.Env = cmdEnv

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("creating window %s in %s: %w", name, session, err)
	}
	windowID := strings.TrimSpace(string(out))

	// Set environment variables inside the tmux window
	for k, v := range env {
		target := session + ":" + windowID
		setCmd := exec.Command("tmux", "send-keys", "-t", target,
			fmt.Sprintf("export %s=%q", k, v), "Enter")
		_ = setCmd.Run()
	}

	// Start Claude
	if claudeCmd != "" {
		time.Sleep(200 * time.Millisecond)
		target := session + ":" + windowID
		startCmd := exec.Command("tmux", "send-keys", "-t", target, claudeCmd, "Enter")
		if err := startCmd.Run(); err != nil {
			return windowID, fmt.Errorf("starting claude in %s: %w", windowID, err)
		}
	}

	return windowID, nil
}

// SendKeys sends literal text followed by Enter to a tmux window.
func SendKeys(session, windowID, keys string) error {
	target := session + ":" + windowID
	cmd := exec.Command("tmux", "send-keys", "-t", target, "-l", keys)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-keys to %s: %s: %w", target, string(out), err)
	}
	return nil
}

// SendEnter sends the Enter key to a tmux window.
func SendEnter(session, windowID string) error {
	target := session + ":" + windowID
	cmd := exec.Command("tmux", "send-keys", "-t", target, "Enter")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-enter to %s: %s: %w", target, string(out), err)
	}
	return nil
}

// SendKeysWithDelay sends text, waits delayMs, then sends Enter.
func SendKeysWithDelay(session, windowID, text string, delayMs int) error {
	if err := SendKeys(session, windowID, text); err != nil {
		return err
	}
	time.Sleep(time.Duration(delayMs) * time.Millisecond)
	return SendEnter(session, windowID)
}

// SendSpecialKey sends a named key (e.g., "Escape", "Up", "Down") to a tmux window.
func SendSpecialKey(session, windowID, key string) error {
	target := session + ":" + windowID
	cmd := exec.Command("tmux", "send-keys", "-t", target, key)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("send-key %s to %s: %s: %w", key, target, string(out), err)
	}
	return nil
}

// CapturePane captures visible pane content.
// If withAnsi is true, includes ANSI escape codes (-e flag) for screenshot rendering.
func CapturePane(session, windowID string, withAnsi bool) (string, error) {
	target := session + ":" + windowID
	args := []string{"capture-pane", "-t", target, "-p"}
	if withAnsi {
		args = append(args, "-e")
	}
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("capturing pane %s: %w", target, err)
	}
	return string(out), nil
}

// KillWindow kills a tmux window. Returns nil if window doesn't exist.
func KillWindow(session, windowID string) error {
	target := session + ":" + windowID
	cmd := exec.Command("tmux", "kill-window", "-t", target)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Ignore error if window is already dead
		outStr := string(out)
		if strings.Contains(outStr, "not found") || strings.Contains(outStr, "no such") ||
			strings.Contains(outStr, "can't find") {
			return nil
		}
		return fmt.Errorf("killing window %s: %s: %w", target, outStr, err)
	}
	return nil
}

// DisplayMessage runs tmux display-message and returns the output.
func DisplayMessage(paneID, format string) (string, error) {
	cmd := exec.Command("tmux", "display-message", "-t", paneID, "-p", format)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("display-message for %s: %w", paneID, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RenameWindow renames a tmux window.
func RenameWindow(session, windowID, newName string) error {
	target := session + ":" + windowID
	cmd := exec.Command("tmux", "rename-window", "-t", target, newName)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("renaming window %s: %s: %w", target, string(out), err)
	}
	return nil
}
