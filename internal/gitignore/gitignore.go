package gitignore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// pierEntries are the lines pier needs in .gitignore
var pierEntries = []string{
	".pier/",
}

// EnsurePierIgnored ensures .pier/ is in .gitignore.
// Creates .gitignore if it doesn't exist.
func EnsurePierIgnored(projectDir string) error {
	gitignorePath := filepath.Join(projectDir, ".gitignore")

	// Check if this is a git repo (don't create .gitignore in non-git dirs)
	if _, err := os.Stat(filepath.Join(projectDir, ".git")); os.IsNotExist(err) {
		// Walk up to find .git (might be in parent for monorepos)
		found := false
		dir := projectDir
		for i := 0; i < 5; i++ {
			if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
				found = true
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
		if !found {
			return nil // Not a git repo, skip
		}
	}

	// Read existing .gitignore
	existing := make(map[string]bool)
	if f, err := os.Open(gitignorePath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			existing[strings.TrimSpace(scanner.Text())] = true
		}
		f.Close()
	}

	// Find missing entries
	var missing []string
	for _, entry := range pierEntries {
		if !existing[entry] {
			missing = append(missing, entry)
		}
	}

	if len(missing) == 0 {
		return nil // Already has all entries
	}

	// Append missing entries
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore: %w", err)
	}
	defer f.Close()

	// Check if file needs a newline before our entries
	info, _ := f.Stat()
	if info != nil && info.Size() > 0 {
		// Read last byte to check for trailing newline
		rf, _ := os.Open(gitignorePath)
		if rf != nil {
			buf := make([]byte, 1)
			rf.Seek(-1, 2)
			rf.Read(buf)
			rf.Close()
			if buf[0] != '\n' {
				fmt.Fprintln(f)
			}
		}
	}

	fmt.Fprintln(f, "\n# Pier (generated files)")
	for _, entry := range missing {
		fmt.Fprintln(f, entry)
	}

	return nil
}
