// Package vault provides lightweight, dependency-free management of the Obsidian
// vault on disk: scanning braindump frontmatter, ensuring the folder layout the
// agent writes into, and archiving processed braindumps.
//
// It deliberately avoids a YAML library — a simple frontmatter line-scan answers
// everything vault-manager needs (the `processed:` flag), keeping go.mod minimal.
package vault

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layout note: vault-manager is deliberately structure-free. The vault's content
// folders (People/, Meetings/, Notes/, …) are owned and described by the vault
// itself (each folder carries a README.md; see AGENTS.md), so the harness neither
// hardcodes nor pre-creates them. It only ensures the two folders it directly
// owns as I/O: the braindump inbox and the processed-braindump archive.

// ArchiveBraindumps is the vault-relative destination for processed braindumps.
const ArchiveBraindumps = "Archive/Braindumps"

// EnsureIODirs creates only the folders the harness itself reads from and writes
// to: the braindump inbox (braindumpDir) and the processed-braindump archive.
// The vault's content structure is self-describing and owned by the vault, so the
// harness intentionally does not create or assume any other folders — the agent
// discovers them by listing the tree and reading folder READMEs.
func EnsureIODirs(vaultPath, braindumpDir string) error {
	for _, dir := range []string{braindumpDir, ArchiveBraindumps} {
		if err := os.MkdirAll(filepath.Join(vaultPath, dir), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	return nil
}

// CountUnprocessed returns how many braindumps still have `processed: false`
// (or a non-truthy `processed:` value). A missing braindump directory is treated
// as zero, not an error.
func CountUnprocessed(vaultPath, braindumpDir string) (int, error) {
	names, err := listMarkdown(filepath.Join(vaultPath, braindumpDir))
	if err != nil {
		return 0, err
	}
	count := 0
	for _, name := range names {
		present, processed, err := frontmatterProcessed(filepath.Join(vaultPath, braindumpDir, name))
		if err != nil {
			return 0, err
		}
		if present && !processed {
			count++
		}
	}
	return count, nil
}

// ArchiveProcessed moves every braindump marked `processed: true` from
// braindumpDir into Archive/Braindumps/ (same filename), and returns the names
// moved. This runs after the agent finishes, so the harness owns the "move to
// archive" step deterministically rather than relying on the agent's tools.
func ArchiveProcessed(vaultPath, braindumpDir string) ([]string, error) {
	srcDir := filepath.Join(vaultPath, braindumpDir)
	names, err := listMarkdown(srcDir)
	if err != nil {
		return nil, err
	}
	dstDir := filepath.Join(vaultPath, ArchiveBraindumps)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, err
	}

	var moved []string
	for _, name := range names {
		present, processed, err := frontmatterProcessed(filepath.Join(srcDir, name))
		if err != nil {
			return moved, err
		}
		if !present || !processed {
			continue
		}
		// Rename within the same filesystem (the vault PVC); atomically replaces
		// any file the agent may already have written at the destination.
		if err := os.Rename(filepath.Join(srcDir, name), filepath.Join(dstDir, name)); err != nil {
			return moved, fmt.Errorf("archiving %s: %w", name, err)
		}
		moved = append(moved, name)
	}
	return moved, nil
}

// listMarkdown returns the .md filenames directly under dir. A missing directory
// yields an empty list (not an error).
func listMarkdown(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// frontmatterProcessed scans a note's leading `---`…`---` YAML frontmatter for a
// `processed:` key. present reports whether the key exists; processed reports
// whether its value is truthy.
func frontmatterProcessed(path string) (present, processed bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return false, false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	if !scanner.Scan() {
		return false, false, scanner.Err()
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return false, false, nil // no frontmatter block
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "---" || line == "..." {
			break // end of frontmatter
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(key), "processed") {
			return true, isTruthy(value), nil
		}
	}
	return false, false, scanner.Err()
}

// isTruthy interprets a frontmatter scalar as a boolean.
func isTruthy(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	v = strings.TrimFunc(v, func(r rune) bool { return r == '"' || r == '\'' })
	switch v {
	case "true", "yes", "on", "1", "done", "processed":
		return true
	default:
		return false
	}
}
