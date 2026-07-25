package agent

import (
	"log/slog"

	copilot "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
)

// shellAllowlist is the set of shell command identifiers the agent may run. The
// Copilot file tools cannot create directories or move files, so the agent needs
// `mkdir` (to scaffold the vault's folders) and `mv` (to archive processed
// braindumps). Everything else stays denied so that untrusted braindump content
// can't coax the agent into running arbitrary commands (e.g. exfiltrating the
// COPILOT_GITHUB_TOKEN).
var shellAllowlist = map[string]bool{
	"mkdir": true,
	"mv":    true,
}

// newPermissionPolicy returns a permission handler that lets the agent read and
// write files and run a tightly-scoped set of shell commands (see
// shellAllowlist), while refusing network access and anything unrecognized.
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
			if shellAllowed(r) {
				log.Info("permission: approved shell (allowlisted)", "command", r.FullCommandText)
				return &rpc.PermissionDecisionApproveOnce{}, nil
			}
			log.Warn("permission: denied shell command", "command", r.FullCommandText)
			return reject("Only `mkdir` (to create vault folders) and `mv` (to archive braindumps) are permitted, with no URLs or output redirection. Use the file tools for everything else."), nil

		case *copilot.PermissionRequestURL:
			log.Warn("permission: denied url fetch", "url", r.URL)
			return reject("Network access is disabled for vault-manager. Work only with the local vault files."), nil

		default:
			log.Warn("permission: denied unrecognized request", "kind", req.Kind())
			return reject("This capability is not permitted for vault-manager. Only reading/writing vault files and `mkdir`/`mv` are allowed."), nil
		}
	}
}

// shellAllowed reports whether a shell request is limited to allowlisted
// commands and touches no URLs or file redirections.
func shellAllowed(r *copilot.PermissionRequestShell) bool {
	if len(r.Commands) == 0 || len(r.PossibleURLs) > 0 || r.HasWriteFileRedirection {
		return false
	}
	for _, c := range r.Commands {
		if !shellAllowlist[c.Identifier] {
			return false
		}
	}
	return true
}

func reject(feedback string) rpc.PermissionDecision {
	return &rpc.PermissionDecisionReject{Feedback: &feedback}
}
