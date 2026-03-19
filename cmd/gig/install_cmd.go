package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func installCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install gig integrations",
	}
	cmd.AddCommand(installHooksCmd())
	return cmd
}

func installHooksCmd() *cobra.Command {
	var (
		flagClaude bool
		flagGit    bool
		flagRoot   bool
	)

	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Install hook scripts for Claude Code and git",
		Long:  "Materializes bundled hook scripts to $GIG_HOME/hooks/ and wires them into Claude Code settings and/or git.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// No flags = both
			if !flagClaude && !flagGit {
				flagClaude = true
				flagGit = true
			}

			gigHome := gig.DefaultGigHome()
			agentDir, gitDir, err := gig.MaterializeHooks(gigHome)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stderr, "Materialized hook scripts to %s/hooks/\n\n", gigHome)

			if flagClaude {
				if err := installClaudeHooks(agentDir, flagRoot); err != nil {
					return fmt.Errorf("install claude hooks: %w", err)
				}
			}
			if flagGit {
				if err := installGitHook(gitDir); err != nil {
					return fmt.Errorf("install git hook: %w", err)
				}
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&flagClaude, "claude", false, "Install Claude Code hooks into settings.json")
	cmd.Flags().BoolVar(&flagGit, "git", false, "Install git post-commit hook symlink")
	cmd.Flags().BoolVar(&flagRoot, "root", false, "With --claude: target ~/.claude/settings.json instead of ./.claude/settings.json")
	return cmd
}

// claudeHookEntry represents one hook entry in Claude Code settings.json.
type claudeHookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// claudeMatcherBlock represents one matcher block in a hook event array.
type claudeMatcherBlock struct {
	Matcher string           `json:"matcher"`
	Hooks   []claudeHookEntry `json:"hooks"`
}

// gigClaudeHooks defines the hooks gig needs installed.
var gigClaudeHooks = []struct {
	Event   string
	Matcher string
	Script  string
}{
	{"SessionStart", "*", "gig-session-start.sh"},
	{"PreCompact", "*", "gig-pre-compact.sh"},
	{"PostToolUse", "Bash", "gig-pickup.sh"},
}

func installClaudeHooks(agentDir string, root bool) error {
	var settingsPath string
	if root {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("get home dir: %w", err)
		}
		settingsPath = filepath.Join(home, ".claude", "settings.json")
	} else {
		settingsPath = filepath.Join(".claude", "settings.json")
	}

	// Read existing settings or start fresh.
	settings := make(map[string]any)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse %s: %w", settingsPath, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read %s: %w", settingsPath, err)
	}

	// Get or create hooks map.
	hooksRaw, _ := settings["hooks"]
	hooks, _ := hooksRaw.(map[string]any)
	if hooks == nil {
		hooks = make(map[string]any)
	}

	for _, h := range gigClaudeHooks {
		scriptPath := filepath.Join(agentDir, h.Script)

		// Get existing event array.
		eventRaw, _ := hooks[h.Event]
		var eventBlocks []any
		if arr, ok := eventRaw.([]any); ok {
			eventBlocks = arr
		}

		// Check if gig hook already present.
		if hasGigHook(eventBlocks, h.Script) {
			continue
		}

		// Add new matcher block.
		block := map[string]any{
			"matcher": h.Matcher,
			"hooks": []any{
				map[string]any{
					"type":    "command",
					"command": scriptPath,
					"timeout": 5,
				},
			},
		}
		eventBlocks = append(eventBlocks, block)
		hooks[h.Event] = eventBlocks
	}

	settings["hooks"] = hooks

	// Write back.
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	out = append(out, '\n')
	if err := os.WriteFile(settingsPath, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", settingsPath, err)
	}

	fmt.Fprintf(os.Stderr, "Installed Claude Code hooks into %s\n", settingsPath)
	for _, h := range gigClaudeHooks {
		fmt.Fprintf(os.Stderr, "  %-14s → %s\n", h.Event, filepath.Join(agentDir, h.Script))
	}
	fmt.Fprintln(os.Stderr)
	return nil
}

// hasGigHook checks whether a gig hook script is already present in an event's matcher blocks.
func hasGigHook(blocks []any, scriptName string) bool {
	for _, b := range blocks {
		bMap, ok := b.(map[string]any)
		if !ok {
			continue
		}
		hooksList, _ := bMap["hooks"].([]any)
		for _, hk := range hooksList {
			hkMap, ok := hk.(map[string]any)
			if !ok {
				continue
			}
			cmd, _ := hkMap["command"].(string)
			if strings.Contains(cmd, scriptName) {
				return true
			}
		}
	}
	return false
}

func installGitHook(gitHookDir string) error {
	// Find git repo root.
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return fmt.Errorf("not inside a git repository")
	}
	repoRoot := strings.TrimSpace(string(out))
	hookPath := filepath.Join(repoRoot, ".git", "hooks", "post-commit")
	target := filepath.Join(gitHookDir, "gig-post-commit.sh")

	// Create .git/hooks/ if missing.
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return fmt.Errorf("create .git/hooks: %w", err)
	}

	// Check if post-commit already exists.
	fi, err := os.Lstat(hookPath)
	if err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			existing, _ := os.Readlink(hookPath)
			if existing == target {
				fmt.Fprintf(os.Stderr, "Git post-commit hook already installed (skipped)\n")
				return nil
			}
		}
		// Real file or symlink to something else — warn and skip.
		fmt.Fprintf(os.Stderr, "Warning: %s already exists, skipping (remove it manually to reinstall)\n", hookPath)
		return nil
	}

	if err := os.Symlink(target, hookPath); err != nil {
		return fmt.Errorf("symlink post-commit: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Installed git post-commit hook\n")
	fmt.Fprintf(os.Stderr, "  %s → %s\n\n", hookPath, target)
	return nil
}
