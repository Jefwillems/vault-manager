package agent

import (
	"log/slog"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// newPermissionPolicy returns a permission handler that lets the agent read and
// write files but denies everything else. The vault harness (vault-manager)
// pre-creates the folder structure and archives processed braindumps itself, so
// the agent never needs shell or network access — which keeps untrusted
// braindump content from coaxing the agent into running commands (e.g.
// exfiltrating the COPILOT_GITHUB_TOKEN).
//
// File writes are constrained by the runtime to the session working directory
// (the vault), so we don't re-implement path containment here.
func newPermissionPolicy(log *slog.Logger) copilot.PermissionHandlerFunc {
	return func(req copilot.PermissionRequest, _ copilot.PermissionInvocation) (rpc.PermissionDecision, error) {
		switch r := req.(type) {
		case *copilot.PermissionRequestRead:
			log.Debug("permission: read", "path", r.Path)
			return &rpc.PermissionDecisionApproveOnce{}, nil

		case *copilot.PermissionRequestWrite:
			log.Info("permission: write", "file", r.FileName)
			return &rpc.PermissionDecisionApproveOnce{}, nil

		case *copilot.PermissionRequestShell:
			log.Warn("permission: denied shell command", "command", r.FullCommandText)
			return reject("Shell access is disabled. The folders you need already exist; use the file tools to read and write notes. You do not need to move files — the harness archives processed braindumps automatically."), nil

		case *copilot.PermissionRequestURL:
			log.Warn("permission: denied url fetch", "url", r.URL)
			return reject("Network access is disabled for vault-manager. Work only with the local vault files."), nil

		default:
			log.Warn("permission: denied unrecognized request", "kind", req.Kind())
			return reject("This capability is not permitted for vault-manager. Only reading and writing vault files is allowed."), nil
		}
	}
}

func reject(feedback string) rpc.PermissionDecision {
	return &rpc.PermissionDecisionReject{Feedback: &feedback}
}
