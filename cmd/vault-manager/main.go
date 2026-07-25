// Command vault-manager is a nightly job that restructures free-form Obsidian
// "braindumps" into a well-organized knowledge base using the GitHub Copilot
// SDK agent runtime. It is intended to run as a Kubernetes CronJob against a
// vault directory kept in sync with CouchDB by livesync-bridge.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"vault-manager/internal/agent"
	"vault-manager/internal/config"
	"vault-manager/internal/vault"
)

func main() {
	os.Exit(run())
}

// run holds the real logic so deferred cleanup runs before the process exits.
// It returns the process exit code (0 success, non-zero failure) so the CronJob
// can report and alert on failed runs.
func run() int {
	cfg, err := config.Load()
	if err != nil {
		// Logger isn't configured yet; use a default stderr logger.
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("configuration error", "error", err)
		return 2
	}

	log := newLogger(cfg.LogLevel)
	log.Info("vault-manager starting",
		"vault", cfg.VaultPath,
		"braindumpDir", cfg.BraindumpDir,
		"model", cfg.Model,
		"reasoningEffort", cfg.ReasoningEffort,
		"timeout", cfg.RunTimeout,
		"force", cfg.Force,
		"extraLayoutDirs", cfg.ExtraLayoutDirs,
	)

	// Pre-flight: skip the (billable) model call when there's nothing to do.
	pending, err := vault.CountUnprocessed(cfg.VaultPath, cfg.BraindumpDir)
	if err != nil {
		log.Error("scanning braindumps", "error", err)
		return 1
	}
	log.Info("braindump scan complete", "unprocessed", pending)
	if pending == 0 && !cfg.Force {
		log.Info("no unprocessed braindumps; nothing to do (set FORCE=true to override)")
		return 0
	}

	// Pre-create the folder skeleton so the agent (which can only read/write
	// files, not create directories) can write notes straight away. Extra dirs
	// from EXTRA_LAYOUT_DIRS let new sections be added without an image rebuild.
	if err := vault.EnsureLayout(cfg.VaultPath, cfg.ExtraLayoutDirs...); err != nil {
		log.Error("ensuring vault layout", "error", err)
		return 1
	}

	// Bound the run and respond to termination signals from Kubernetes.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.RunTimeout)
	defer cancel()
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	if err := agent.Run(ctx, log, cfg); err != nil {
		log.Error("run failed", "error", err)
		return 1
	}

	// Deterministically archive the braindumps the agent marked processed,
	// rather than relying on the agent's (shell-less) tools to move files.
	moved, err := vault.ArchiveProcessed(cfg.VaultPath, cfg.BraindumpDir)
	if err != nil {
		log.Error("archiving processed braindumps", "error", err)
		return 1
	}
	log.Info("archived processed braindumps", "count", len(moved), "files", moved)

	log.Info("vault-manager finished")
	return 0
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
