package bot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFileBrowser_DirsFirstThenFiles(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "zdir"), 0o755)
	os.Mkdir(filepath.Join(dir, "adir"), 0o755)
	os.WriteFile(filepath.Join(dir, "bfile.txt"), []byte("hi"), 0o644)
	os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("hi"), 0o644)

	_, _, entries := buildFileBrowser(dir, 0)

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
	// Dirs first (sorted), then files (sorted)
	expected := []struct {
		name  string
		isDir bool
	}{
		{"adir", true},
		{"zdir", true},
		{"afile.txt", false},
		{"bfile.txt", false},
	}
	for i, e := range expected {
		if entries[i].Name != e.name || entries[i].IsDir != e.isDir {
			t.Errorf("entry %d: got {%s, %v}, want {%s, %v}",
				i, entries[i].Name, entries[i].IsDir, e.name, e.isDir)
		}
	}
}

func TestBuildFileBrowser_HiddenEntriesExcluded(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, ".hidden"), 0o755)
	os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(""), 0o644)
	os.Mkdir(filepath.Join(dir, "visible"), 0o755)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0o644)

	_, _, entries := buildFileBrowser(dir, 0)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name, ".") {
			t.Errorf("hidden entry should be excluded: %s", e.Name)
		}
	}
}

func TestBuildFileBrowser_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()

	text, kb, entries := buildFileBrowser(dir, 0)

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
	if !strings.Contains(text, "empty directory") {
		t.Errorf("text should mention empty directory, got: %s", text)
	}
	// Should still have action row (.. | Cancel)
	if len(kb.InlineKeyboard) == 0 {
		t.Error("keyboard should have action row")
	}
}

func TestBuildFileBrowser_InvalidPath(t *testing.T) {
	text, _, entries := buildFileBrowser("/nonexistent/path/xyz", 0)

	if entries != nil {
		t.Error("entries should be nil for invalid path")
	}
	if text == "" {
		t.Error("should return error text")
	}
	if !strings.Contains(text, "Error") {
		t.Errorf("text should mention error, got: %s", text)
	}
}

func TestBuildFileBrowser_Pagination(t *testing.T) {
	dir := t.TempDir()
	// Create 10 files to exceed filesPerPage (8)
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i))+".txt"), []byte("hi"), 0o644)
	}

	_, kb, entries := buildFileBrowser(dir, 0)

	if len(entries) != 10 {
		t.Fatalf("expected 10 entries, got %d", len(entries))
	}

	// Should have a next page button
	found := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && *btn.CallbackData == "get_page:1" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected next page button (get_page:1)")
	}

	// Page 1 should show remaining entries and have a back button
	_, kb2, _ := buildFileBrowser(dir, 1)
	hasBack := false
	for _, row := range kb2.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && *btn.CallbackData == "get_page:0" {
				hasBack = true
			}
		}
	}
	if !hasBack {
		t.Error("page 2 should have back button (get_page:0)")
	}
}

func TestBuildFileBrowser_NoPaginationWhenFewEntries(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hi"), 0o644)

	_, kb, _ := buildFileBrowser(dir, 0)

	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && strings.HasPrefix(*btn.CallbackData, "get_page:") {
				t.Error("should not have pagination buttons with few entries")
			}
		}
	}
}

func TestBuildFileBrowser_PageBounds(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)

	// Page -1 should clamp to 0
	_, _, entries := buildFileBrowser(dir, -1)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	// Page 999 should clamp to last page
	_, _, entries = buildFileBrowser(dir, 999)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestBuildFileBrowser_TwoButtonsPerRow(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 4; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i))+".txt"), []byte("hi"), 0o644)
	}

	_, kb, _ := buildFileBrowser(dir, 0)

	// First two rows should be entry buttons with 2 per row
	// Last row is the action row (..|Cancel)
	if len(kb.InlineKeyboard) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(kb.InlineKeyboard))
	}
	for i := 0; i < 2; i++ {
		if len(kb.InlineKeyboard[i]) != 2 {
			t.Errorf("row %d: expected 2 buttons, got %d", i, len(kb.InlineKeyboard[i]))
		}
	}
}

func TestBuildFileBrowser_OddEntryCount(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 3; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i))+".txt"), []byte("hi"), 0o644)
	}

	_, kb, _ := buildFileBrowser(dir, 0)

	// Row 0: 2 buttons, Row 1: 1 button (odd), then action row
	if len(kb.InlineKeyboard) < 3 {
		t.Fatalf("expected at least 3 rows, got %d", len(kb.InlineKeyboard))
	}
	if len(kb.InlineKeyboard[0]) != 2 {
		t.Errorf("row 0: expected 2 buttons, got %d", len(kb.InlineKeyboard[0]))
	}
	if len(kb.InlineKeyboard[1]) != 1 {
		t.Errorf("row 1: expected 1 button (odd entry), got %d", len(kb.InlineKeyboard[1]))
	}
}

func TestBuildFileBrowser_DirEmoji(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hi"), 0o644)

	_, kb, _ := buildFileBrowser(dir, 0)

	// First entry row should have dir with folder emoji and file without
	row := kb.InlineKeyboard[0]
	if len(row) != 2 {
		t.Fatalf("expected 2 buttons in first row, got %d", len(row))
	}
	if !strings.HasPrefix(row[0].Text, "\U0001F4C1") {
		t.Errorf("dir button should have folder emoji, got: %s", row[0].Text)
	}
	if strings.HasPrefix(row[1].Text, "\U0001F4C1") {
		t.Errorf("file button should not have folder emoji, got: %s", row[1].Text)
	}
}

func TestBuildFileBrowser_CallbackDataFormat(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644)

	_, kb, _ := buildFileBrowser(dir, 0)

	// Entry buttons use get_sel:<index> format
	row := kb.InlineKeyboard[0]
	if row[0].CallbackData == nil || *row[0].CallbackData != "get_sel:0" {
		t.Errorf("first button callback: got %v, want get_sel:0", row[0].CallbackData)
	}
	if row[1].CallbackData == nil || *row[1].CallbackData != "get_sel:1" {
		t.Errorf("second button callback: got %v, want get_sel:1", row[1].CallbackData)
	}
}

func TestBuildFileBrowser_ActionRow(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)

	_, kb, _ := buildFileBrowser(dir, 0)

	lastRow := kb.InlineKeyboard[len(kb.InlineKeyboard)-1]
	if len(lastRow) != 2 {
		t.Fatalf("action row should have 2 buttons, got %d", len(lastRow))
	}
	expected := []string{"get_up", "get_cancel"}
	for i, btn := range lastRow {
		if btn.CallbackData == nil || *btn.CallbackData != expected[i] {
			t.Errorf("action button %d: got %v, want %s", i, btn.CallbackData, expected[i])
		}
	}
}

func TestBuildFileBrowser_HeaderText(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "sub1"), 0o755)
	os.Mkdir(filepath.Join(dir, "sub2"), 0o755)
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644)

	text, _, _ := buildFileBrowser(dir, 0)

	if !strings.Contains(text, "2 dirs") {
		t.Errorf("header should show 2 dirs, got: %s", text)
	}
	if !strings.Contains(text, "1 files") {
		t.Errorf("header should show 1 files, got: %s", text)
	}
}

func TestBuildFileBrowser_FollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "realdir")
	os.Mkdir(realDir, 0o755)
	os.Symlink(realDir, filepath.Join(dir, "linkdir"))

	_, _, entries := buildFileBrowser(dir, 0)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Both should be dirs (symlink resolved)
	for _, e := range entries {
		if !e.IsDir {
			t.Errorf("entry %s should be a directory (symlink followed)", e.Name)
		}
	}
}

func TestBuildFileBrowser_PageIndicator(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		os.WriteFile(filepath.Join(dir, "file"+string(rune('a'+i))+".txt"), []byte("hi"), 0o644)
	}

	_, kb, _ := buildFileBrowser(dir, 0)

	// Find the noop page indicator button showing "1/2"
	found := false
	for _, row := range kb.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData != nil && *btn.CallbackData == "get_noop" && btn.Text == "1/2" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected page indicator button showing 1/2")
	}
}
