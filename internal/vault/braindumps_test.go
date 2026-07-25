package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCountUnprocessed(t *testing.T) {
	vaultPath := t.TempDir()
	braindumpDir := "Braindumps"
	dir := filepath.Join(vaultPath, braindumpDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"unprocessed.md":    "---\ntype: braindump\nprocessed: false\n---\n\nnotes",
		"processed.md":      "---\ntype: braindump\nprocessed: true\n---\n\nnotes",
		"quoted.md":         "---\nprocessed: \"false\"\n---\n",
		"missing-key.md":    "---\ntype: braindump\n---\nno processed key -> not counted",
		"no-frontmatter.md": "just a note without frontmatter",
		"not-markdown.txt":  "---\nprocessed: false\n---\n",
		"yaml-yes.md":       "---\nprocessed: yes\n---\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A nested directory should be ignored.
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := CountUnprocessed(vaultPath, braindumpDir)
	if err != nil {
		t.Fatalf("CountUnprocessed: %v", err)
	}
	// unprocessed.md + quoted.md = 2
	if want := 2; got != want {
		t.Errorf("CountUnprocessed = %d, want %d", got, want)
	}
}

func TestCountUnprocessedMissingDir(t *testing.T) {
	got, err := CountUnprocessed(t.TempDir(), "DoesNotExist")
	if err != nil {
		t.Fatalf("expected no error for missing dir, got %v", err)
	}
	if got != 0 {
		t.Errorf("CountUnprocessed = %d, want 0", got)
	}
}
