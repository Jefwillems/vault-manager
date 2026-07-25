// Package config loads vault-manager runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for a vault-manager run.
type Config struct {
	// VaultPath is the absolute path to the vault working directory (the bridge
	// storage baseDir). The Copilot agent operates with this as its cwd.
	VaultPath string
	// BraindumpDir is the vault-relative directory that holds raw braindumps.
	BraindumpDir string
	// Model is the Copilot model to use for the session.
	Model string
	// ReasoningEffort is passed to models that support it ("low"|"medium"|"high"|"xhigh").
	ReasoningEffort string
	// GitHubToken authenticates against Copilot (requires an active subscription).
	GitHubToken string
	// LogLevel controls slog verbosity ("debug"|"info"|"warn"|"error").
	LogLevel string
	// RunTimeout bounds the wall-clock time of a single run.
	RunTimeout time.Duration
	// Force runs the agent even when no unprocessed braindumps are found.
	Force bool
	// ExtraLayoutDirs are additional vault-relative folders to pre-create beyond
	// the built-in Layout, so new sections can be added by editing the CronJob
	// (and AGENTS.md) without rebuilding the image. Sourced from EXTRA_LAYOUT_DIRS
	// as a comma-separated list.
	ExtraLayoutDirs []string
}

// Load reads configuration from the environment, applying defaults, and
// validates required fields.
func Load() (*Config, error) {
	cfg := &Config{
		VaultPath:    env("VAULT_PATH", "/app/data/vault"),
		BraindumpDir: env("BRAINDUMP_DIR", "Braindumps"),
		Model:        env("COPILOT_MODEL", "claude-sonnet-4.5"),
		// Empty by default: many models (including claude-sonnet-4.5 via the
		// Copilot API) reject an explicit reasoning-effort setting. Set this only
		// for models where models.list reports reasoningEffort support.
		ReasoningEffort: os.Getenv("COPILOT_REASONING_EFFORT"),
		GitHubToken:     os.Getenv("COPILOT_GITHUB_TOKEN"),
		LogLevel:        env("LOG_LEVEL", "info"),
	}

	timeout, err := parseDuration(env("RUN_TIMEOUT", "30m"))
	if err != nil {
		return nil, fmt.Errorf("RUN_TIMEOUT: %w", err)
	}
	cfg.RunTimeout = timeout

	force, err := parseBool(os.Getenv("FORCE"))
	if err != nil {
		return nil, fmt.Errorf("FORCE: %w", err)
	}
	cfg.Force = force

	extraDirs, err := parseLayoutDirs(os.Getenv("EXTRA_LAYOUT_DIRS"))
	if err != nil {
		return nil, fmt.Errorf("EXTRA_LAYOUT_DIRS: %w", err)
	}
	cfg.ExtraLayoutDirs = extraDirs

	if cfg.GitHubToken == "" {
		return nil, fmt.Errorf("COPILOT_GITHUB_TOKEN is required")
	}
	if cfg.VaultPath == "" {
		return nil, fmt.Errorf("VAULT_PATH must not be empty")
	}

	return cfg, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(v string) (time.Duration, error) {
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, err
	}
	if d <= 0 {
		return 0, fmt.Errorf("must be positive, got %q", v)
	}
	return d, nil
}

func parseBool(v string) (bool, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return false, nil
	}
	return strconv.ParseBool(v)
}

// parseLayoutDirs splits a comma-separated list of vault-relative folders,
// trimming blanks. Each entry must stay inside the vault: absolute paths and
// `..` escapes are rejected so a bad env var can't make the harness create
// directories outside the vault root.
func parseLayoutDirs(v string) ([]string, error) {
	var dirs []string
	for _, part := range strings.Split(v, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		clean := filepath.Clean(p)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("invalid layout dir %q: must be a relative path inside the vault", p)
		}
		dirs = append(dirs, clean)
	}
	return dirs, nil
}
