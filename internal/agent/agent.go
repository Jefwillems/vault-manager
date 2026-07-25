// Package agent drives the GitHub Copilot SDK agent runtime against the vault.
//
// vault-manager is a thin orchestrator: it starts the embedded Copilot runtime
// with the vault as its working directory, opens a session governed by a
// restrictive permission policy (file operations allowed, shell/network denied),
// sends a single processing instruction, and waits for the agent to go idle.
// All of the actual editing intelligence lives in the model plus the vault's
// AGENTS.md contract.
package agent

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	copilot "github.com/github/copilot-sdk/go"

	"vault-manager/internal/config"
)

//go:embed system.md
var systemPrompt string

// instruction is the single per-run message sent to the agent. The heavy
// lifting (schema, folder layout, rules) lives in system.md and the vault's
// AGENTS.md; this just kicks off the run.
const instruction = `Process the vault now.

1. Read AGENTS.md at the vault root for the structure, schemas, and rules.
2. Find every note under the braindumps folder whose frontmatter has ` + "`processed: false`" + `.
3. For each one, fold its content into the proper notes (People, Meetings, Actions, ADR, Notes),
   updating existing notes in place rather than creating duplicates, and linking entities with [[wikilinks]].
4. Mark each processed braindump ` + "`processed: true`" + ` and move it to the archive folder.
5. Write a concise summary of everything you changed to a dated file in the History folder.

If there are no unprocessed braindumps, do nothing and stop.`

// Run executes one full agent pass over the vault. It returns nil when the agent
// completed its turn without a fatal session error, or an error describing the
// first fatal condition encountered.
func Run(ctx context.Context, log *slog.Logger, cfg *config.Config) error {
	client := copilot.NewClient(&copilot.ClientOptions{
		WorkingDirectory: cfg.VaultPath,
		GitHubToken:      cfg.GitHubToken,
		LogLevel:         runtimeLogLevel(cfg.LogLevel),
	})

	if err := client.Start(ctx); err != nil {
		return fmt.Errorf("start copilot runtime: %w", err)
	}
	defer func() {
		if err := client.Stop(); err != nil {
			log.Warn("stopping copilot runtime", "error", err)
		}
	}()

	session, err := client.CreateSession(ctx, &copilot.SessionConfig{
		Model:               cfg.Model,
		ReasoningEffort:     cfg.ReasoningEffort,
		WorkingDirectory:    cfg.VaultPath,
		OnPermissionRequest: newPermissionPolicy(log),
		SystemMessage:       &copilot.SystemMessageConfig{Content: systemPrompt},
	})
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	defer func() {
		if err := session.Disconnect(); err != nil {
			log.Warn("disconnecting session", "error", err)
		}
	}()

	var (
		mu     sync.Mutex
		runErr error
	)
	idle := make(chan struct{}, 1)

	unsub := session.On(func(e copilot.SessionEvent) {
		switch d := e.Data.(type) {
		case *copilot.AssistantMessageData:
			if content := strings.TrimSpace(d.Content); content != "" {
				log.Info("assistant", "message", content)
			}
		case *copilot.SessionErrorData:
			log.Error("session error", "type", d.ErrorType, "message", d.Message)
			if isFatal(d.ErrorType) {
				mu.Lock()
				if runErr == nil {
					runErr = fmt.Errorf("session error (%s): %s", d.ErrorType, d.Message)
				}
				mu.Unlock()
			}
		case *copilot.SessionIdleData:
			select {
			case idle <- struct{}{}:
			default:
			}
		}
	})
	defer unsub()

	if _, err := session.Send(ctx, copilot.MessageOptions{Prompt: instruction}); err != nil {
		return fmt.Errorf("send instruction: %w", err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("run cancelled before completion: %w", ctx.Err())
	case <-idle:
	}

	mu.Lock()
	defer mu.Unlock()
	return runErr
}

// isFatal reports whether a session error type means the run cannot succeed.
// Transient categories (e.g. rate_limit, which the runtime may auto-recover
// from) are logged but not treated as terminal.
func isFatal(errorType string) bool {
	switch errorType {
	case "authentication", "authorization", "quota", "context_limit":
		return true
	default:
		return false
	}
}

// runtimeLogLevel maps our slog-style level onto the runtime's accepted values.
func runtimeLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return "debug"
	case "warn", "warning":
		return "warning"
	case "error":
		return "error"
	default:
		return "info"
	}
}
