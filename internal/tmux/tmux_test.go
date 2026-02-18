package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

func hasTmux() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func skipWithoutTmux(t *testing.T) {
	t.Helper()
	if !hasTmux() {
		t.Skip("tmux not available")
	}
}

const testSession = "tramuntana_test"

func cleanupTestSession(t *testing.T) {
	t.Helper()
	exec.Command("tmux", "kill-session", "-t", testSession).Run()
}

func TestSessionExists_NonExistent(t *testing.T) {
	skipWithoutTmux(t)
	if SessionExists("nonexistent_session_xyz_12345") {
		t.Error("expected false for non-existent session")
	}
}

func TestEnsureSession_And_ListWindows(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	if !SessionExists(testSession) {
		t.Fatal("session should exist after EnsureSession")
	}

	// Calling again should be a no-op
	err = EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession (idempotent): %v", err)
	}

	windows, err := ListWindows(testSession)
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}
	if len(windows) == 0 {
		t.Fatal("expected at least one window")
	}
	if windows[0].ID == "" {
		t.Error("window ID should not be empty")
	}
}

func TestNewWindow_And_SendKeys(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windowID, err := NewWindow(testSession, "testwin", "/tmp", "", nil)
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}
	if windowID == "" {
		t.Fatal("window ID should not be empty")
	}

	err = SendKeys(testSession, windowID, "echo hello")
	if err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	err = SendEnter(testSession, windowID)
	if err != nil {
		t.Fatalf("SendEnter: %v", err)
	}
}

func TestCapturePane(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windows, _ := ListWindows(testSession)
	if len(windows) == 0 {
		t.Fatal("no windows")
	}

	// Plain text capture
	text, err := CapturePane(testSession, windows[0].ID, false)
	if err != nil {
		t.Fatalf("CapturePane (plain): %v", err)
	}
	_ = text // just verify no error

	// ANSI capture
	ansi, err := CapturePane(testSession, windows[0].ID, true)
	if err != nil {
		t.Fatalf("CapturePane (ansi): %v", err)
	}
	_ = ansi
}

func TestSendKeysWithDelay(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windows, _ := ListWindows(testSession)
	if len(windows) == 0 {
		t.Fatal("no windows")
	}

	// Use 10ms delay for test speed
	err = SendKeysWithDelay(testSession, windows[0].ID, "echo test", 10)
	if err != nil {
		t.Fatalf("SendKeysWithDelay: %v", err)
	}
}

func TestSendSpecialKey(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windows, _ := ListWindows(testSession)
	if len(windows) == 0 {
		t.Fatal("no windows")
	}

	err = SendSpecialKey(testSession, windows[0].ID, "Escape")
	if err != nil {
		t.Fatalf("SendSpecialKey: %v", err)
	}
}

func TestKillWindow(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windowID, err := NewWindow(testSession, "tokill", "/tmp", "", nil)
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	err = KillWindow(testSession, windowID)
	if err != nil {
		t.Fatalf("KillWindow: %v", err)
	}

	// Killing again should not error (already dead)
	err = KillWindow(testSession, windowID)
	if err != nil {
		t.Fatalf("KillWindow (idempotent): %v", err)
	}
}

func TestRenameWindow(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windowID, err := NewWindow(testSession, "oldname", "/tmp", "", nil)
	if err != nil {
		t.Fatalf("NewWindow: %v", err)
	}

	err = RenameWindow(testSession, windowID, "newname")
	if err != nil {
		t.Fatalf("RenameWindow: %v", err)
	}

	windows, _ := ListWindows(testSession)
	found := false
	for _, w := range windows {
		if w.ID == windowID && w.Name == "newname" {
			found = true
			break
		}
	}
	if !found {
		t.Error("window was not renamed")
	}
}

func TestDisplayMessage(t *testing.T) {
	skipWithoutTmux(t)
	cleanupTestSession(t)
	defer cleanupTestSession(t)

	err := EnsureSession(testSession)
	if err != nil {
		t.Fatalf("EnsureSession: %v", err)
	}

	windows, _ := ListWindows(testSession)
	if len(windows) == 0 {
		t.Fatal("no windows")
	}

	// Use the window target for display-message
	target := testSession + ":" + windows[0].ID
	result, err := DisplayMessage(target, "#{session_name}:#{window_id}")
	if err != nil {
		t.Fatalf("DisplayMessage: %v", err)
	}

	if !strings.Contains(result, testSession) {
		t.Errorf("result %q should contain session name %q", result, testSession)
	}
}
