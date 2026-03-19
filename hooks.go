package gig

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed hooks/agent/*.sh hooks/git/*.sh
var HookFS embed.FS

// MaterializeHooks writes the embedded hook scripts to gigHome/hooks/.
// Returns the absolute paths to the agent and git hook directories.
// Idempotent: always overwrites with the version from the binary.
func MaterializeHooks(gigHome string) (agentDir, gitDir string, err error) {
	agentDir = filepath.Join(gigHome, "hooks", "agent")
	gitDir = filepath.Join(gigHome, "hooks", "git")

	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create agent hooks dir: %w", err)
	}
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create git hooks dir: %w", err)
	}

	err = fs.WalkDir(HookFS, "hooks", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		data, err := HookFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}

		dest := filepath.Join(gigHome, path)
		if err := os.WriteFile(dest, data, 0o755); err != nil {
			return fmt.Errorf("write %s: %w", dest, err)
		}
		return nil
	})
	if err != nil {
		return "", "", fmt.Errorf("materialize hooks: %w", err)
	}

	return agentDir, gitDir, nil
}
