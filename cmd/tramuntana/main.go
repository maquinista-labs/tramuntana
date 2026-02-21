package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/otaviocarvalho/tramuntana/hook"
	"github.com/otaviocarvalho/tramuntana/internal/bot"
	"github.com/otaviocarvalho/tramuntana/internal/config"
	"github.com/otaviocarvalho/tramuntana/internal/monitor"
	"github.com/otaviocarvalho/tramuntana/internal/queue"
	"github.com/otaviocarvalho/tramuntana/internal/state"
	"github.com/spf13/cobra"
)

var (
	version     = "v0.1.0"
	cfgPath     string
	cfg         *config.Config
	installHook bool
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "tramuntana",
		Short: "Bridge Telegram group topics to Claude Code sessions via tmux",
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Telegram bot",
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if cfgPath != "" {
				_ = godotenv.Load(cfgPath)
			}
			var err error
			cfg, err = config.Load()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe()
		},
	}
	serveCmd.Flags().StringVar(&cfgPath, "config", "", "path to .env config file")

	hookCmd := &cobra.Command{
		Use:   "hook",
		Short: "Run the Claude Code SessionStart hook",
		RunE: func(cmd *cobra.Command, args []string) error {
			if installHook {
				return hook.Install()
			}
			return hook.Run()
		},
	}
	hookCmd.Flags().BoolVar(&installHook, "install", false, "install hook into Claude Code settings")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("tramuntana %s\n", version)
		},
	}

	rootCmd.AddCommand(serveCmd, hookCmd, versionCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe() error {
	// Create bot
	b, err := bot.New(cfg)
	if err != nil {
		return fmt.Errorf("creating bot: %w", err)
	}

	// Load monitor state
	msPath := filepath.Join(cfg.TramuntanaDir, "monitor_state.json")
	ms, err := state.LoadMonitorState(msPath)
	if err != nil {
		log.Printf("Warning: loading monitor state: %v (starting fresh)", err)
		ms = state.NewMonitorState()
	}
	b.SetMonitorState(ms)

	// Startup recovery: reconcile state with live tmux windows
	liveBindings := b.ReconcileState()
	log.Printf("Startup: %d live bindings recovered", liveBindings)

	// Create message queue
	q := queue.New(b.API())

	// Create session monitor
	mon := monitor.New(cfg, b.State(), ms, q)

	// Create status poller
	sp := bot.NewStatusPoller(b, q, mon)

	// Context for graceful shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Start monitor in background
	go mon.Run(ctx)

	// Start status poller in background
	go sp.Run(ctx)

	// Run bot (blocks until ctx is cancelled)
	err = b.Run(ctx)

	// Graceful shutdown: save all state
	log.Println("Saving state...")
	if err := ms.ForceSave(msPath); err != nil {
		log.Printf("Error saving monitor state: %v", err)
	}

	return err
}
