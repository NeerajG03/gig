//go:build e2e

package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

var (
	gigBinary string
	buildOnce sync.Once
	buildErr  error
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gig-e2e-*")
	if err != nil {
		panic(err)
	}
	gigBinary = filepath.Join(dir, "gig")
	buildErr = exec.Command("go", "build", "-o", gigBinary, ".").Run()

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func buildGig(t *testing.T) string {
	t.Helper()
	if buildErr != nil {
		t.Fatalf("build gig: %v", buildErr)
	}
	return gigBinary
}

func setupGig(t *testing.T) (bin string, home string) {
	t.Helper()
	bin = buildGig(t)
	home = t.TempDir()
	run(t, bin, home, "init", "--prefix", "test")
	return bin, home
}

func run(t *testing.T, bin, home string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "GIG_HOME="+home, "NO_COLOR=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gig %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func runExpectFail(t *testing.T, bin, home string, args ...string) string {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), "GIG_HOME="+home, "NO_COLOR=1")
	out, _ := cmd.CombinedOutput()
	return string(out)
}

func assertContains(t *testing.T, output, substr string) {
	t.Helper()
	if !strings.Contains(output, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, output)
	}
}

func assertNotContains(t *testing.T, output, substr string) {
	t.Helper()
	if strings.Contains(output, substr) {
		t.Errorf("expected output to NOT contain %q, got:\n%s", substr, output)
	}
}

func TestCLI_Init(t *testing.T) {
	bin := buildGig(t)
	home := t.TempDir()
	out := run(t, bin, home, "init", "--prefix", "myapp")
	assertContains(t, out, "Initialized")
	assertContains(t, out, "myapp")

	if _, err := os.Stat(filepath.Join(home, "gig.yaml")); err != nil {
		t.Error("config file not created")
	}
	if _, err := os.Stat(filepath.Join(home, "gig.db")); err != nil {
		t.Error("database file not created")
	}
}

func TestCLI_InitAlreadyExists(t *testing.T) {
	bin, home := setupGig(t)
	out := run(t, bin, home, "init")
	assertContains(t, out, "already initialized")
}

func TestCLI_CreateAndShow(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Fix login bug", "--type", "bug", "--priority", "1", "--quiet"))

	show := run(t, bin, home, "show", id)
	assertContains(t, show, "Fix login bug")
	assertContains(t, show, "bug")
	assertContains(t, show, id)
}

func TestCLI_CreateWithParent(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Epic task", "--type", "epic", "--quiet"))
	childID := strings.TrimSpace(run(t, bin, home, "create", "Subtask", "--parent", parentID, "--quiet"))

	if !strings.HasPrefix(childID, parentID+".") {
		t.Errorf("child ID %q should start with %q", childID, parentID+".")
	}
}

func TestCLI_List(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "create", "Task A", "--priority", "0")
	run(t, bin, home, "create", "Task B", "--priority", "2")
	run(t, bin, home, "create", "Task C", "--priority", "1")

	out := run(t, bin, home, "list")
	assertContains(t, out, "Task A")
	assertContains(t, out, "Task B")
	assertContains(t, out, "Task C")
}

func TestCLI_ListTree(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Parent task", "--quiet"))
	run(t, bin, home, "create", "Child task", "--parent", parentID)

	out := run(t, bin, home, "list", "--tree")
	assertContains(t, out, "Parent task")
	assertContains(t, out, "Child task")
	// Tree connectors should be present.
	if !strings.Contains(out, "└──") && !strings.Contains(out, "├──") {
		t.Error("expected tree connectors in output")
	}
}

func TestCLI_ListAll(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Closeable task", "--quiet"))
	run(t, bin, home, "close", id)

	without := run(t, bin, home, "list")
	assertNotContains(t, without, "Closeable task")

	with := run(t, bin, home, "list", "--all")
	assertContains(t, with, "Closeable task")
}

func TestCLI_TreeClosedParentOpenChild(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Parent epic", "--type", "epic", "--quiet"))
	run(t, bin, home, "create", "Open child", "--parent", parentID)

	// Closing parent with open children should fail.
	cmd := exec.Command(bin, "--actor", "test", "close", parentID)
	cmd.Env = append(os.Environ(), "GIG_HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected close to fail with open child")
	}
	assertContains(t, string(out), "close or cancel all children first")

	// Parent stays open in tree with its open child.
	treeOut := run(t, bin, home, "list", "--tree")
	assertContains(t, treeOut, "Parent epic")
	assertContains(t, treeOut, "Open child")
}

func TestCLI_TreeClosedParentClosedChildren(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Dead epic", "--type", "epic", "--quiet"))
	childID := strings.TrimSpace(run(t, bin, home, "create", "Done child", "--parent", parentID, "--quiet"))
	run(t, bin, home, "close", childID)
	run(t, bin, home, "close", parentID)

	// Default tree: fully closed subtree should be hidden.
	out := run(t, bin, home, "list", "--tree")
	assertNotContains(t, out, "Dead epic")
	assertNotContains(t, out, "Done child")

	// With --all: both should appear.
	out = run(t, bin, home, "list", "--tree", "--all")
	assertContains(t, out, "Dead epic")
	assertContains(t, out, "Done child")
}

func TestCLI_TreeCancelledHiddenByDefault(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Cancel epic", "--type", "epic", "--quiet"))
	run(t, bin, home, "create", "Cancel child", "--parent", parentID)
	run(t, bin, home, "cancel", parentID)

	// Default tree: cancelled subtree should be hidden.
	out := run(t, bin, home, "list", "--tree")
	assertNotContains(t, out, "Cancel epic")
	assertNotContains(t, out, "Cancel child")

	// With --all: both should appear.
	out = run(t, bin, home, "list", "--tree", "--all")
	assertContains(t, out, "Cancel epic")
	assertContains(t, out, "Cancel child")
}

func TestCLI_TreeDeepOpenDescendant(t *testing.T) {
	bin, home := setupGig(t)

	epicID := strings.TrimSpace(run(t, bin, home, "create", "Top epic", "--type", "epic", "--quiet"))
	midID := strings.TrimSpace(run(t, bin, home, "create", "Mid task", "--parent", epicID, "--quiet"))
	run(t, bin, home, "create", "Leaf task", "--parent", midID)

	// Closing mid should fail because leaf is open.
	cmd := exec.Command(bin, "--actor", "test", "close", midID)
	cmd.Env = append(os.Environ(), "GIG_HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected close mid to fail with open leaf")
	}
	assertContains(t, string(out), "close or cancel all children first")

	// All three should show in tree (all still open).
	treeOut := run(t, bin, home, "list", "--tree")
	assertContains(t, treeOut, "Top epic")
	assertContains(t, treeOut, "Mid task")
	assertContains(t, treeOut, "Leaf task")
}

func TestCLI_Close(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "To close", "--quiet"))
	out := run(t, bin, home, "close", id, "--reason", "done")
	assertContains(t, out, "Closed")

	show := run(t, bin, home, "show", id)
	assertContains(t, show, "closed")
	assertContains(t, show, "done")
}

func TestCLI_Reopen(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Reopen me", "--quiet"))
	run(t, bin, home, "close", id)
	run(t, bin, home, "reopen", id)

	show := run(t, bin, home, "show", id)
	assertContains(t, show, "open")
}

func TestCLI_Search(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "create", "Unique needle task")
	run(t, bin, home, "create", "Something else")

	out := run(t, bin, home, "search", "needle")
	assertContains(t, out, "Unique needle task")
	assertNotContains(t, out, "Something else")
}

func TestCLI_Comment(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Commentable", "--quiet"))
	run(t, bin, home, "comment", id, "Test note here")

	out := run(t, bin, home, "comments", id)
	assertContains(t, out, "Test note here")
}

func TestCLI_Dep(t *testing.T) {
	bin, home := setupGig(t)

	idA := strings.TrimSpace(run(t, bin, home, "create", "Blocker", "--quiet"))
	idB := strings.TrimSpace(run(t, bin, home, "create", "Blocked task", "--quiet"))

	run(t, bin, home, "dep", "add", idB, idA)

	out := run(t, bin, home, "dep", "list", idB)
	assertContains(t, out, idA)
}

func TestCLI_Ready(t *testing.T) {
	bin, home := setupGig(t)

	idBlocker := strings.TrimSpace(run(t, bin, home, "create", "Blocker task", "--quiet"))
	idBlocked := strings.TrimSpace(run(t, bin, home, "create", "Blocked task", "--quiet"))
	run(t, bin, home, "dep", "add", idBlocked, idBlocker)

	out := run(t, bin, home, "ready")
	assertContains(t, out, "Blocker task")
	assertNotContains(t, out, "Blocked task")
}

func TestCLI_CancelCascadesChildren(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Parent to cancel", "--quiet"))
	child1ID := strings.TrimSpace(run(t, bin, home, "create", "Child one", "--parent", parentID, "--quiet"))
	child2ID := strings.TrimSpace(run(t, bin, home, "create", "Child two", "--parent", parentID, "--quiet"))

	run(t, bin, home, "cancel", parentID)

	// All should be cancelled.
	for _, id := range []string{parentID, child1ID, child2ID} {
		out := run(t, bin, home, "show", id, "--json")
		assertContains(t, out, "cancelled")
	}
}

func TestCLI_CloseBlockedByOpenChild(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Parent to close", "--quiet"))
	run(t, bin, home, "create", "Open child", "--parent", parentID)

	// Close should fail.
	cmd := exec.Command(bin, "--actor", "test", "close", parentID)
	cmd.Env = append(os.Environ(), "GIG_HOME="+home)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected close to fail with open child")
	}
	assertContains(t, string(out), "close or cancel all children first")
}

func TestCLI_CloseSucceedsWhenChildrenClosed(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Closable parent", "--quiet"))
	childID := strings.TrimSpace(run(t, bin, home, "create", "Closable child", "--parent", parentID, "--quiet"))

	run(t, bin, home, "close", childID)
	run(t, bin, home, "close", parentID)

	out := run(t, bin, home, "show", parentID, "--json")
	assertContains(t, out, "closed")
}

func TestCLI_ReadyExcludesBlockedParentChildren(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Blocked parent", "--quiet"))
	run(t, bin, home, "create", "Child of blocked", "--parent", parentID)

	// Block the parent via a dependency.
	blockerID := strings.TrimSpace(run(t, bin, home, "create", "Blocker", "--quiet"))
	run(t, bin, home, "dep", "add", parentID, blockerID)

	// Parent is now blocked. Child should not appear in ready.
	out := run(t, bin, home, "ready")
	assertNotContains(t, out, "Child of blocked")
	assertContains(t, out, "Blocker")
}

func TestCLI_ReadyExcludesCancelledParentChildren(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Cancelled parent", "--quiet"))
	run(t, bin, home, "create", "Orphan child", "--parent", parentID)

	run(t, bin, home, "cancel", parentID)

	// Child is cascade-cancelled so should not appear in ready.
	out := run(t, bin, home, "ready")
	assertNotContains(t, out, "Orphan child")
}

func TestCLI_ReadyTreeView(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Epic", "--type", "epic", "--quiet"))
	run(t, bin, home, "create", "Ready subtask", "--parent", parentID)

	// Default ready should show tree with parent context.
	out := run(t, bin, home, "ready")
	assertContains(t, out, "Epic")
	assertContains(t, out, "Ready subtask")
	// Tree connectors should be present.
	assertContains(t, out, "└──")
}

func TestCLI_ReadyListFlag(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Epic", "--type", "epic", "--quiet"))
	run(t, bin, home, "create", "Ready subtask", "--parent", parentID)

	// --list should show flat output without tree connectors.
	out := run(t, bin, home, "ready", "--list")
	assertContains(t, out, "Ready subtask")
	assertNotContains(t, out, "└──")
}

func TestCLI_ConfigSet(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "config", "set", "default_view", "tree")

	out := run(t, bin, home, "config")
	assertContains(t, out, "tree")
}

func TestCLI_JSONOutput(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "create", "JSON task")

	out := run(t, bin, home, "list", "--json")
	var tasks []map[string]any
	if err := json.Unmarshal([]byte(out), &tasks); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(tasks) == 0 {
		t.Error("expected at least 1 task in JSON output")
	}
}

func TestCLI_QuietOutput(t *testing.T) {
	bin, home := setupGig(t)

	out := run(t, bin, home, "create", "Quiet task", "--quiet")
	id := strings.TrimSpace(out)

	if id == "" {
		t.Fatal("quiet output should return task ID")
	}
	if strings.Contains(out, "Created") {
		t.Error("quiet output should not contain 'Created'")
	}
}

func TestCLI_Actor(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Actor test", "--actor", "agent-1", "--quiet"))

	out := run(t, bin, home, "events", id)
	assertContains(t, out, "agent-1")
}

func TestCLI_Completion(t *testing.T) {
	bin, home := setupGig(t)

	out := run(t, bin, home, "completion", "bash")
	if len(out) < 100 {
		t.Error("bash completion script seems too short")
	}

	out = run(t, bin, home, "completion", "zsh")
	if len(out) < 100 {
		t.Error("zsh completion script seems too short")
	}
}

func TestCLI_Stats(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "create", "Task 1")
	run(t, bin, home, "create", "Task 2")

	out := run(t, bin, home, "stats")
	assertContains(t, out, "Total:")
	assertContains(t, out, "2")
}

func TestCLI_Update(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Original title", "--quiet"))
	run(t, bin, home, "update", id, "--title", "Updated title")

	show := run(t, bin, home, "show", id)
	assertContains(t, show, "Updated title")
}

func TestCLI_Children(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "Parent", "--type", "epic", "--quiet"))
	run(t, bin, home, "create", "Child 1", "--parent", parentID)
	run(t, bin, home, "create", "Child 2", "--parent", parentID)

	out := run(t, bin, home, "children", parentID)
	assertContains(t, out, "Child 1")
	assertContains(t, out, "Child 2")
}

func TestCLI_Doctor(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "create", "Task 1")
	run(t, bin, home, "create", "Task 2")

	out := run(t, bin, home, "doctor")
	assertContains(t, out, "Checking gig health")
	assertContains(t, out, "[ok]")
	assertContains(t, out, "database integrity OK")
	assertContains(t, out, "no dependency cycles")
	assertContains(t, out, "2 task(s) in database")
	assertContains(t, out, "All checks passed")
}

func TestCLI_DoctorJSON(t *testing.T) {
	bin, home := setupGig(t)

	out := run(t, bin, home, "doctor", "--json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	diags, ok := report["diagnostics"].([]any)
	if !ok || len(diags) == 0 {
		t.Error("expected diagnostics array in JSON output")
	}
}

func TestCLI_DoctorEmptyDB(t *testing.T) {
	bin, home := setupGig(t)

	out := run(t, bin, home, "doctor")
	assertContains(t, out, "0 task(s) in database")
	assertContains(t, out, "All checks passed")
}

func TestCLI_Blocked(t *testing.T) {
	bin, home := setupGig(t)

	idBlocker := strings.TrimSpace(run(t, bin, home, "create", "Blocker", "--quiet"))
	idBlocked := strings.TrimSpace(run(t, bin, home, "create", "Blocked one", "--quiet"))
	run(t, bin, home, "dep", "add", idBlocked, idBlocker)

	out := run(t, bin, home, "blocked")
	assertContains(t, out, "Blocked one")
	assertNotContains(t, out, "Blocker")
}

func TestCLI_ExportImport(t *testing.T) {
	bin, home := setupGig(t)

	run(t, bin, home, "create", "Export me", "--priority", "1")

	exportPath := filepath.Join(home, "export.jsonl")
	run(t, bin, home, "export", "--file", exportPath)

	if _, err := os.Stat(exportPath); err != nil {
		t.Fatalf("export file not created: %v", err)
	}

	// Create a fresh GIG_HOME and import.
	home2 := t.TempDir()
	run(t, bin, home2, "init", "--prefix", "test")
	run(t, bin, home2, "import", "--file", exportPath)

	out := run(t, bin, home2, "list")
	assertContains(t, out, "Export me")
}

func TestCLI_InvalidCommand(t *testing.T) {
	bin, home := setupGig(t)

	out := runExpectFail(t, bin, home, "nonexistent")
	assertContains(t, out, "unknown command")
}

func TestCLI_ConfigSetInvalidKey(t *testing.T) {
	bin, home := setupGig(t)

	out := runExpectFail(t, bin, home, "config", "set", "fake_key", "value")
	assertContains(t, out, "unknown config key")
}

func TestCLI_ConfigSetInvalidValue(t *testing.T) {
	bin, home := setupGig(t)

	out := runExpectFail(t, bin, home, "config", "set", "default_view", "kanban")
	assertContains(t, out, "list")
	assertContains(t, out, "tree")

	out = runExpectFail(t, bin, home, "config", "set", "hash_length", "99")
	assertContains(t, out, "3")
	assertContains(t, out, "8")
}

func TestCLI_DepCycle(t *testing.T) {
	bin, home := setupGig(t)

	idA := strings.TrimSpace(run(t, bin, home, "create", "A", "--quiet"))
	idB := strings.TrimSpace(run(t, bin, home, "create", "B", "--quiet"))
	run(t, bin, home, "dep", "add", idB, idA)

	// Adding reverse dep should fail (cycle).
	out := runExpectFail(t, bin, home, "dep", "add", idA, idB)
	assertContains(t, out, "cycle")
}

// Regression tests for gig-33da: --status filter in tree view should match
// subtasks, not just root tasks.

func TestCLI_TreeStatusFilterMatchesSubtask(t *testing.T) {
	bin, home := setupGig(t)

	// Parent is open, subtask is in_progress.
	parentID := strings.TrimSpace(run(t, bin, home, "create", "Open parent", "--quiet"))
	childID := strings.TrimSpace(run(t, bin, home, "create", "WIP subtask", "--parent", parentID, "--quiet"))
	run(t, bin, home, "update", childID, "--status", "in_progress")

	// --status in_progress should surface the subtask (and its parent for context).
	out := run(t, bin, home, "list", "--tree", "--status", "in_progress")
	assertContains(t, out, "Open parent")
	assertContains(t, out, "WIP subtask")
}

func TestCLI_TreeStatusFilterExcludesNonMatching(t *testing.T) {
	bin, home := setupGig(t)

	// Two root tasks, both open. One has an in_progress subtask.
	parentA := strings.TrimSpace(run(t, bin, home, "create", "Parent A", "--quiet"))
	childA := strings.TrimSpace(run(t, bin, home, "create", "Child A WIP", "--parent", parentA, "--quiet"))
	run(t, bin, home, "update", childA, "--status", "in_progress")

	run(t, bin, home, "create", "Parent B standalone")

	// Only the tree containing an in_progress node should appear.
	out := run(t, bin, home, "list", "--tree", "--status", "in_progress")
	assertContains(t, out, "Parent A")
	assertContains(t, out, "Child A WIP")
	assertNotContains(t, out, "Parent B standalone")
}

func TestCLI_TreeStatusFilterDeepDescendant(t *testing.T) {
	bin, home := setupGig(t)

	// 3-level tree: epic -> mid -> leaf (in_progress).
	epicID := strings.TrimSpace(run(t, bin, home, "create", "Epic", "--type", "epic", "--quiet"))
	midID := strings.TrimSpace(run(t, bin, home, "create", "Mid task", "--parent", epicID, "--quiet"))
	leafID := strings.TrimSpace(run(t, bin, home, "create", "Deep leaf", "--parent", midID, "--quiet"))
	run(t, bin, home, "update", leafID, "--status", "in_progress")

	out := run(t, bin, home, "list", "--tree", "--status", "in_progress")
	assertContains(t, out, "Epic")
	assertContains(t, out, "Mid task")
	assertContains(t, out, "Deep leaf")
}

func TestCLI_TreeStatusFilterJSON(t *testing.T) {
	bin, home := setupGig(t)

	parentID := strings.TrimSpace(run(t, bin, home, "create", "JSON parent", "--quiet"))
	childID := strings.TrimSpace(run(t, bin, home, "create", "JSON child WIP", "--parent", parentID, "--quiet"))
	run(t, bin, home, "update", childID, "--status", "in_progress")

	out := run(t, bin, home, "list", "--tree", "--status", "in_progress", "--json")
	var trees []map[string]any
	if err := json.Unmarshal([]byte(out), &trees); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(trees) == 0 {
		t.Fatal("expected at least 1 tree in JSON output")
	}
	// The root should have children containing the matching subtask.
	children, ok := trees[0]["children"].([]any)
	if !ok || len(children) == 0 {
		t.Error("expected root task to have children in JSON tree")
	}
}

func TestCLI_ClaimTask(t *testing.T) {
	bin, home := setupGig(t)

	id := strings.TrimSpace(run(t, bin, home, "create", "Claimable", "--quiet"))
	run(t, bin, home, "update", id, "--claim")

	show := run(t, bin, home, "show", id)
	assertContains(t, show, "in_progress")
}
