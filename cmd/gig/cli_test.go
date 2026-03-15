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
