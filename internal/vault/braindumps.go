// Package vault provides lightweight, dependency-free inspection of the Obsidian
// vault on disk. It deliberately avoids a YAML library: the only thing
// vault-manager needs before invoking the agent is a count of unprocessed
// braindumps, which a simple frontmatter line-scan answers reliably.
package vault

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// CountUnprocessed scans braindumpDir (relative to vaultPath) for Markdown files
// whose YAML frontmatter contains `processed: false`, and returns how many were
// found. A missing braindump directory is treated as zero (not an error): the
// vault may simply not have been populated yet.
func CountUnprocessed(vaultPath, braindumpDir string) (int, error) {
	dir := filepath.Join(vaultPath, braindumpDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}
		unprocessed, err := isUnprocessed(filepath.Join(dir, entry.Name()))
		if err != nil {
			return 0, err
		}
		if unprocessed {
			count++
		}
	}
	return count, nil
}

// isUnprocessed reports whether the file has a leading `---`…`---` frontmatter
// block containing a `processed:` key set to a falsey value.
func isUnprocessed(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// The frontmatter block must be the very first non-empty content and open
	// with a `---` fence.
	if !scanner.Scan() {
		return false, scanner.Err()
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return false, nil
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
			return isFalsey(value), nil
		}
	}
	return false, scanner.Err()
}

// isFalsey interprets a frontmatter scalar as a boolean-ish "not processed"
// signal. Anything that isn't clearly truthy counts as unprocessed so a
// malformed dump errs toward being picked up rather than silently skipped.
func isFalsey(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	// Strip surrounding quotes and trailing inline comments.
	v = strings.TrimFunc(v, func(r rune) bool { return r == '"' || r == '\'' })
	switch v {
	case "true", "yes", "on", "1", "done", "processed":
		return false
	default:
		return true
	}
}
