package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	TelegramBotToken    string
	AllowedUsers        []int64
	AllowedGroups       []int64
	TramuntanaDir       string
	TmuxSessionName     string
	ClaudeCommand       string
	MonitorPollInterval float64
	MinuanoBin          string
	MinuanoDB           string
	MinuanoScriptsDir   string
	QueueTopicID        int64
	ApprovalsTopicID    int64
	DefaultProject      string
}

func Load(envFile ...string) (*Config, error) {
	for _, f := range envFile {
		_ = godotenv.Load(f)
	}
	_ = godotenv.Load() // default .env, ignore if missing

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	usersStr := os.Getenv("ALLOWED_USERS")
	if usersStr == "" {
		return nil, fmt.Errorf("ALLOWED_USERS is required")
	}
	users, err := parseIntList(usersStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ALLOWED_USERS: %w", err)
	}

	var groups []int64
	if g := os.Getenv("ALLOWED_GROUPS"); g != "" {
		groups, err = parseIntList(g)
		if err != nil {
			return nil, fmt.Errorf("invalid ALLOWED_GROUPS: %w", err)
		}
	}

	dir := os.Getenv("TRAMUNTANA_DIR")
	if dir == "" {
		dir = "~/.tramuntana"
	}
	dir = expandHome(dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating tramuntana dir: %w", err)
	}

	sessionName := os.Getenv("TMUX_SESSION_NAME")
	if sessionName == "" {
		sessionName = "tramuntana"
	}

	claudeCmd := os.Getenv("CLAUDE_COMMAND")
	if claudeCmd == "" {
		claudeCmd = "claude"
	}

	pollInterval := 2.0
	if p := os.Getenv("MONITOR_POLL_INTERVAL"); p != "" {
		pollInterval, err = strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid MONITOR_POLL_INTERVAL: %w", err)
		}
	}

	minuanoBin := os.Getenv("MINUANO_BIN")
	if minuanoBin == "" {
		minuanoBin = "minuano"
	}

	minuanoScriptsDir := os.Getenv("MINUANO_SCRIPTS_DIR")

	var queueTopicID int64
	if q := os.Getenv("TRAMUNTANA_QUEUE_TOPIC_ID"); q != "" {
		queueTopicID, _ = strconv.ParseInt(q, 10, 64)
	}

	var approvalsTopicID int64
	if a := os.Getenv("TRAMUNTANA_APPROVALS_TOPIC_ID"); a != "" {
		approvalsTopicID, _ = strconv.ParseInt(a, 10, 64)
	}

	defaultProject := os.Getenv("TRAMUNTANA_DEFAULT_PROJECT")

	return &Config{
		TelegramBotToken:    token,
		AllowedUsers:        users,
		AllowedGroups:       groups,
		TramuntanaDir:       dir,
		TmuxSessionName:     sessionName,
		ClaudeCommand:       claudeCmd,
		MonitorPollInterval: pollInterval,
		MinuanoBin:          minuanoBin,
		MinuanoDB:           os.Getenv("MINUANO_DB"),
		MinuanoScriptsDir:   minuanoScriptsDir,
		QueueTopicID:        queueTopicID,
		ApprovalsTopicID:    approvalsTopicID,
		DefaultProject:      defaultProject,
	}, nil
}

func (c *Config) IsAllowedUser(userID int64) bool {
	for _, id := range c.AllowedUsers {
		if id == userID {
			return true
		}
	}
	return false
}

func (c *Config) IsAllowedGroup(groupID int64) bool {
	if len(c.AllowedGroups) == 0 {
		return true // no restriction if not configured
	}
	for _, id := range c.AllowedGroups {
		if id == groupID {
			return true
		}
	}
	return false
}

func parseIntList(s string) ([]int64, error) {
	var result []int64
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing %q: %w", part, err)
		}
		result = append(result, n)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("empty list")
	}
	return result, nil
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
