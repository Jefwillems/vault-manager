package vault

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeVaultFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCountUnprocessed(t *testing.T) {
	vaultPath := t.TempDir()
	braindumpDir := "Braindumps"
	writeVaultFiles(t, filepath.Join(vaultPath, braindumpDir), map[string]string{
		"unprocessed.md":    "---\ntype: braindump\nprocessed: false\n---\n\nnotes",
		"processed.md":      "---\ntype: braindump\nprocessed: true\n---\n\nnotes",
		"quoted.md":         "---\nprocessed: \"false\"\n---\n",
		"missing-key.md":    "---\ntype: braindump\n---\nno processed key",
		"no-frontmatter.md": "just a note",
		"not-markdown.txt":  "---\nprocessed: false\n---\n",
		"yaml-yes.md":       "---\nprocessed: yes\n---\n",
	})

	got, err := CountUnprocessed(vaultPath, braindumpDir)
	if err != nil {
		t.Fatalf("CountUnprocessed: %v", err)
	}
	if want := 2; got != want { // unprocessed.md + quoted.md
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

func TestEnsureLayout(t *testing.T) {
	vaultPath := t.TempDir()
	if err := EnsureLayout(vaultPath); err != nil {
		t.Fatalf("EnsureLayout: %v", err)
	}
	for _, dir := range Layout {
		if fi, err := os.Stat(filepath.Join(vaultPath, dir)); err != nil || !fi.IsDir() {
			t.Errorf("expected directory %s to exist", dir)
		}
	}
	// Idempotent.
	if err := EnsureLayout(vaultPath); err != nil {
		t.Fatalf("EnsureLayout second call: %v", err)
	}
}

func TestArchiveProcessed(t *testing.T) {
	vaultPath := t.TempDir()
	braindumpDir := "Braindumps"
	writeVaultFiles(t, filepath.Join(vaultPath, braindumpDir), map[string]string{
		"done-1.md": "---\ntype: braindump\nprocessed: true\n---\ncontent",
		"done-2.md": "---\nprocessed: true\n---\ncontent",
		"open.md":   "---\nprocessed: false\n---\ncontent",
		"nokey.md":  "---\ntype: braindump\n---\ncontent",
	})

	moved, err := ArchiveProcessed(vaultPath, braindumpDir)
	if err != nil {
		t.Fatalf("ArchiveProcessed: %v", err)
	}
	sort.Strings(moved)
	if want := []string{"done-1.md", "done-2.md"}; len(moved) != 2 || moved[0] != want[0] || moved[1] != want[1] {
		t.Fatalf("moved = %v, want %v", moved, want)
	}

	// Processed files are now in the archive, not the source.
	for _, name := range moved {
		if _, err := os.Stat(filepath.Join(vaultPath, braindumpDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s should have been moved out of Braindumps", name)
		}
		if _, err := os.Stat(filepath.Join(vaultPath, ArchiveBraindumps, name)); err != nil {
			t.Errorf("%s should exist in archive: %v", name, err)
		}
	}
	// Unprocessed / keyless files stay put.
	for _, name := range []string{"open.md", "nokey.md"} {
		if _, err := os.Stat(filepath.Join(vaultPath, braindumpDir, name)); err != nil {
			t.Errorf("%s should remain in Braindumps: %v", name, err)
		}
	}
}
