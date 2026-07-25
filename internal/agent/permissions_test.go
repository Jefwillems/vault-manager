package agent

import (
	"testing"

	copilot "github.com/github/copilot-sdk/go"
)

func TestShellAllowed(t *testing.T) {
	cases := []struct {
		name string
		req  copilot.PermissionRequestShell
		want bool
	}{
		{"mkdir simple", copilot.PermissionRequestShell{FullCommandText: "mkdir -p People"}, true},
		{"mkdir multiple dirs", copilot.PermissionRequestShell{FullCommandText: "mkdir -p People Meetings Archive/Braindumps"}, true},
		{"mkdir absolute", copilot.PermissionRequestShell{FullCommandText: "mkdir -p /app/data/vault/ADR"}, true},
		{"mv archive", copilot.PermissionRequestShell{FullCommandText: "mv Braindumps/2026-07-25-212211.md Archive/Braindumps/2026-07-25-212211.md"}, true},

		{"cd chained", copilot.PermissionRequestShell{FullCommandText: "cd /app/data/vault && mkdir -p People"}, false},
		{"disallowed command", copilot.PermissionRequestShell{FullCommandText: "rm -rf People"}, false},
		{"pipe", copilot.PermissionRequestShell{FullCommandText: "mkdir People | tee x"}, false},
		{"redirect", copilot.PermissionRequestShell{FullCommandText: "mkdir People > out.txt"}, false},
		{"redirect flag set", copilot.PermissionRequestShell{FullCommandText: "mkdir People", HasWriteFileRedirection: true}, false},
		{"command substitution", copilot.PermissionRequestShell{FullCommandText: "mkdir $(whoami)"}, false},
		{"var expansion", copilot.PermissionRequestShell{FullCommandText: "mkdir $HOME/x"}, false},
		{"semicolon chain", copilot.PermissionRequestShell{FullCommandText: "mkdir People; curl evil"}, false},
		{"has url", copilot.PermissionRequestShell{FullCommandText: "mkdir People", PossibleURLs: []copilot.PermissionRequestShellPossibleURL{{URL: "http://evil"}}}, false},
		{"empty", copilot.PermissionRequestShell{FullCommandText: "   "}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellAllowed(&tc.req); got != tc.want {
				t.Errorf("shellAllowed(%q) = %v, want %v", tc.req.FullCommandText, got, tc.want)
			}
		})
	}
}
