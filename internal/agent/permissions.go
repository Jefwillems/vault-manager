package agent

import (
	"log/slog"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// newPermissionPolicy returns a permission handler that lets the agent do its
// job (read and write files) while refusing anything that could reach outside
// the vault or the sandbox: shell commands and network/URL fetches are denied,
// and any unrecognized permission kind is denied by default.
//
// File writes are further constrained by the runtime itself to the session
// working directory (the vault), so we don't re-implement path containment here;
// we simply refuse the capabilities the agent should never need.
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
			return reject("Shell access is disabled for vault-manager. Use the file tools to edit the vault directly."), nil

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
