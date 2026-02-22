package minuano

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Bridge communicates with the Minuano CLI.
type Bridge struct {
	Bin    string // path to minuano binary
	DBFlag string // optional --db flag value
}

// NewBridge creates a new Bridge with the given binary path and optional DB flag.
func NewBridge(bin, dbFlag string) *Bridge {
	return &Bridge{Bin: bin, DBFlag: dbFlag}
}

// Task represents a Minuano task (matches minuano's JSON output).
type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Status      string     `json:"status"`
	Priority    int        `json:"priority"`
	Capability  *string    `json:"capability,omitempty"`
	ClaimedBy   *string    `json:"claimed_by,omitempty"`
	ProjectID   *string    `json:"project_id,omitempty"`
	Attempt     int        `json:"attempt"`
	MaxAttempts int        `json:"max_attempts"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

// TaskContext represents a context entry for a task.
type TaskContext struct {
	ID         int64      `json:"id"`
	TaskID     string     `json:"task_id"`
	AgentID    *string    `json:"agent_id,omitempty"`
	Kind       string     `json:"kind"`
	Content    string     `json:"content"`
	SourceTask *string    `json:"source_task,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
}

// TaskDetail holds a task with its context entries.
type TaskDetail struct {
	Task    *Task          `json:"task"`
	Context []*TaskContext `json:"context"`
}

// run executes a minuano command and returns stdout.
func (b *Bridge) run(args ...string) (string, error) {
	if b.DBFlag != "" {
		args = append([]string{"--db", b.DBFlag}, args...)
	}

	cmd := exec.Command(b.Bin, args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("minuano %s: %s", strings.Join(args, " "), string(exitErr.Stderr))
		}
		return "", fmt.Errorf("minuano %s: %w", strings.Join(args, " "), err)
	}

	return string(out), nil
}

// Status returns the task list for a project (or all tasks if project is empty).
func (b *Bridge) Status(project string) ([]Task, error) {
	args := []string{"status", "--json"}
	if project != "" {
		args = append(args, "--project", project)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	var tasks []Task
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		return nil, fmt.Errorf("parsing status JSON: %w", err)
	}

	return tasks, nil
}

// Show returns detailed info for a specific task.
func (b *Bridge) Show(taskID string) (*TaskDetail, error) {
	out, err := b.run("show", "--json", taskID)
	if err != nil {
		return nil, err
	}

	var detail TaskDetail
	if err := json.Unmarshal([]byte(out), &detail); err != nil {
		return nil, fmt.Errorf("parsing show JSON: %w", err)
	}

	return &detail, nil
}

// Tree returns the dependency tree as raw text.
func (b *Bridge) Tree(project string) (string, error) {
	args := []string{"tree"}
	if project != "" {
		args = append(args, "--project", project)
	}

	out, err := b.run(args...)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(out, "\n"), nil
}

// Prompt generates a self-contained prompt for the given mode.
func (b *Bridge) Prompt(mode string, args ...string) (string, error) {
	cmdArgs := append([]string{"prompt", mode}, args...)
	out, err := b.run(cmdArgs...)
	if err != nil {
		return "", err
	}

	return strings.TrimRight(out, "\n"), nil
}

// PromptSingle generates a single-task prompt.
func (b *Bridge) PromptSingle(taskID string) (string, error) {
	return b.Prompt("single", taskID)
}

// PromptAuto generates an auto-mode loop prompt.
func (b *Bridge) PromptAuto(project string) (string, error) {
	return b.Prompt("auto", "--project", project)
}

// PromptBatch generates a batch prompt for multiple tasks.
func (b *Bridge) PromptBatch(taskIDs ...string) (string, error) {
	return b.Prompt("batch", taskIDs...)
}

// Delete removes a task by ID using a direct SQL delete via psql.
func (b *Bridge) Delete(taskID string) error {
	if b.DBFlag == "" {
		return fmt.Errorf("DATABASE_URL not configured")
	}

	cmd := exec.Command("psql", b.DBFlag, "-c",
		fmt.Sprintf("DELETE FROM tasks WHERE id = '%s'", sanitizeID(taskID)))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("deleting task %s: %s", taskID, strings.TrimSpace(string(out)))
	}

	// psql outputs "DELETE N" — check that exactly 1 row was deleted
	output := strings.TrimSpace(string(out))
	if output != "DELETE 1" {
		return fmt.Errorf("task %s not found", taskID)
	}

	return nil
}

// sanitizeID strips everything except alphanumerics and hyphens from a task ID.
func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// AddResult holds the output of a successful task creation.
type AddResult struct {
	ID    string
	Title string
}

// Add creates a new task via `minuano add`.
func (b *Bridge) Add(title, project, body string, priority int) (*AddResult, error) {
	args := []string{"add", title, "--project", project, "--priority", strconv.Itoa(priority)}
	if body != "" {
		args = append(args, "--body", body)
	}

	out, err := b.run(args...)
	if err != nil {
		return nil, err
	}

	return parseAddOutput(out)
}

// parseAddOutput extracts the task ID and title from `minuano add` output.
// Expected format: "Created: <id>  \"<title>\"\n"
func parseAddOutput(out string) (*AddResult, error) {
	line := strings.TrimSpace(out)
	if !strings.HasPrefix(line, "Created: ") {
		return nil, fmt.Errorf("unexpected add output: %s", line)
	}

	rest := line[len("Created: "):]
	// ID and title are separated by double-space
	idx := strings.Index(rest, "  ")
	if idx < 0 {
		return nil, fmt.Errorf("unexpected add output (no double-space separator): %s", line)
	}

	id := rest[:idx]
	title := strings.Trim(rest[idx+2:], "\"")

	return &AddResult{ID: id, Title: title}, nil
}
