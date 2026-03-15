package gig

import (
	"testing"
)

func TestCreateAndGet(t *testing.T) {
	store, _ := tempDB(t)

	task, err := store.Create(CreateParams{
		Title:       "Test task",
		Description: "A test",
		Type:        TypeTask,
		Priority:    P1,
		Assignee:    "neeraj",
		Labels:      []string{"backend"},
		CreatedBy:   "test",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if task.Title != "Test task" {
		t.Errorf("title = %q, want 'Test task'", task.Title)
	}
	if task.Status != StatusOpen {
		t.Errorf("status = %q, want 'open'", task.Status)
	}

	got, err := store.Get(task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != task.Title {
		t.Errorf("got title %q, want %q", got.Title, task.Title)
	}
	if got.Assignee != "neeraj" {
		t.Errorf("assignee = %q, want 'neeraj'", got.Assignee)
	}
}

func TestCreateRequiresTitle(t *testing.T) {
	store, _ := tempDB(t)
	_, err := store.Create(CreateParams{})
	if err == nil {
		t.Error("expected error for empty title")
	}
}

func TestCreateWithParent(t *testing.T) {
	store, _ := tempDB(t)

	parent, _ := store.Create(CreateParams{Title: "Parent", Type: TypeEpic})
	child, err := store.Create(CreateParams{Title: "Child", ParentID: parent.ID})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if child.ParentID != parent.ID {
		t.Errorf("parent_id = %q, want %q", child.ParentID, parent.ID)
	}
}

func TestCreateWithInvalidParent(t *testing.T) {
	store, _ := tempDB(t)
	_, err := store.Create(CreateParams{Title: "Child", ParentID: "nonexistent"})
	if err == nil {
		t.Error("expected error for invalid parent")
	}
}

func TestUpdate(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Original"})

	newTitle := "Updated"
	updated, err := store.Update(task.ID, UpdateParams{Title: &newTitle}, "test")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Title != "Updated" {
		t.Errorf("title = %q, want 'Updated'", updated.Title)
	}
}

func TestUpdateStatus(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Task"})

	if err := store.UpdateStatus(task.ID, StatusInProgress, "test"); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := store.Get(task.ID)
	if got.Status != StatusInProgress {
		t.Errorf("status = %q, want 'in_progress'", got.Status)
	}
}

func TestCloseAndReopen(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Task"})

	if err := store.CloseTask(task.ID, "done", "test"); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, _ := store.Get(task.ID)
	if got.Status != StatusClosed {
		t.Errorf("status = %q, want 'closed'", got.Status)
	}
	if got.CloseReason != "done" {
		t.Errorf("close_reason = %q, want 'done'", got.CloseReason)
	}
	if got.ClosedAt == nil {
		t.Error("closed_at should be set")
	}

	if err := store.Reopen(task.ID, "test"); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	got, _ = store.Get(task.ID)
	if got.Status != StatusOpen {
		t.Errorf("status after reopen = %q, want 'open'", got.Status)
	}
}

func TestCloseMany(t *testing.T) {
	store, _ := tempDB(t)
	t1, _ := store.Create(CreateParams{Title: "Task 1"})
	t2, _ := store.Create(CreateParams{Title: "Task 2"})

	if err := store.CloseMany([]string{t1.ID, t2.ID}, "batch", "test"); err != nil {
		t.Fatalf("close many: %v", err)
	}

	g1, _ := store.Get(t1.ID)
	g2, _ := store.Get(t2.ID)
	if g1.Status != StatusClosed || g2.Status != StatusClosed {
		t.Error("both tasks should be closed")
	}
}

func TestClaim(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Task"})

	if err := store.Claim(task.ID, "agent-1"); err != nil {
		t.Fatalf("claim: %v", err)
	}

	got, _ := store.Get(task.ID)
	if got.Assignee != "agent-1" {
		t.Errorf("assignee = %q, want 'agent-1'", got.Assignee)
	}
	if got.Status != StatusInProgress {
		t.Errorf("status = %q, want 'in_progress'", got.Status)
	}
}

func TestList(t *testing.T) {
	store, _ := tempDB(t)
	store.Create(CreateParams{Title: "A", Priority: P0, Assignee: "neeraj"})
	store.Create(CreateParams{Title: "B", Priority: P2, Assignee: "jeff"})
	store.Create(CreateParams{Title: "C", Priority: P1, Assignee: "neeraj"})

	// List all
	all, err := store.List(ListParams{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 tasks, got %d", len(all))
	}

	// Filter by assignee
	neerajTasks, _ := store.List(ListParams{Assignee: "neeraj"})
	if len(neerajTasks) != 2 {
		t.Errorf("expected 2 tasks for neeraj, got %d", len(neerajTasks))
	}

	// Filter by status
	status := StatusOpen
	openTasks, _ := store.List(ListParams{Status: &status})
	if len(openTasks) != 3 {
		t.Errorf("expected 3 open tasks, got %d", len(openTasks))
	}

	// Limit
	limited, _ := store.List(ListParams{Limit: 2})
	if len(limited) != 2 {
		t.Errorf("expected 2 tasks with limit, got %d", len(limited))
	}
}

func TestSearch(t *testing.T) {
	store, _ := tempDB(t)
	store.Create(CreateParams{Title: "Fix login bug"})
	store.Create(CreateParams{Title: "Add search feature", Description: "full-text search"})
	store.Create(CreateParams{Title: "Update docs"})

	results, err := store.Search("search")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'search', got %d", len(results))
	}

	results, _ = store.Search("bug")
	if len(results) != 1 {
		t.Errorf("expected 1 result for 'bug', got %d", len(results))
	}
}

func TestChildren(t *testing.T) {
	store, _ := tempDB(t)
	parent, _ := store.Create(CreateParams{Title: "Epic", Type: TypeEpic})
	store.Create(CreateParams{Title: "Sub 1", ParentID: parent.ID})
	store.Create(CreateParams{Title: "Sub 2", ParentID: parent.ID})

	children, err := store.Children(parent.ID)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	if len(children) != 2 {
		t.Errorf("expected 2 children, got %d", len(children))
	}
}

func TestSubtaskIDLadder(t *testing.T) {
	store, _ := tempDB(t)
	parent, _ := store.Create(CreateParams{Title: "Epic", Type: TypeEpic})

	c1, _ := store.Create(CreateParams{Title: "Sub 1", ParentID: parent.ID})
	c2, _ := store.Create(CreateParams{Title: "Sub 2", ParentID: parent.ID})
	c3, _ := store.Create(CreateParams{Title: "Sub 3", ParentID: parent.ID})

	// Subtask IDs should be parent.1, parent.2, parent.3
	if c1.ID != parent.ID+".1" {
		t.Errorf("child1 ID = %q, want %q", c1.ID, parent.ID+".1")
	}
	if c2.ID != parent.ID+".2" {
		t.Errorf("child2 ID = %q, want %q", c2.ID, parent.ID+".2")
	}
	if c3.ID != parent.ID+".3" {
		t.Errorf("child3 ID = %q, want %q", c3.ID, parent.ID+".3")
	}

	// Grandchildren should also ladder: parent.1.1, parent.1.2
	gc1, _ := store.Create(CreateParams{Title: "Grandchild 1", ParentID: c1.ID})
	gc2, _ := store.Create(CreateParams{Title: "Grandchild 2", ParentID: c1.ID})

	if gc1.ID != c1.ID+".1" {
		t.Errorf("grandchild1 ID = %q, want %q", gc1.ID, c1.ID+".1")
	}
	if gc2.ID != c1.ID+".2" {
		t.Errorf("grandchild2 ID = %q, want %q", gc2.ID, c1.ID+".2")
	}
}

func TestGetTree(t *testing.T) {
	store, _ := tempDB(t)
	root, _ := store.Create(CreateParams{Title: "Root"})
	child, _ := store.Create(CreateParams{Title: "Child", ParentID: root.ID})
	store.Create(CreateParams{Title: "Grandchild", ParentID: child.ID})

	tree, err := store.GetTree(root.ID)
	if err != nil {
		t.Fatalf("get tree: %v", err)
	}
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Children))
	}
	if len(tree.Children[0].Children) != 1 {
		t.Errorf("expected 1 grandchild, got %d", len(tree.Children[0].Children))
	}
}

func TestReadyAndBlocked(t *testing.T) {
	store, _ := tempDB(t)
	blocker, _ := store.Create(CreateParams{Title: "Blocker"})
	blocked, _ := store.Create(CreateParams{Title: "Blocked"})
	free, _ := store.Create(CreateParams{Title: "Free"})

	store.AddDependency(blocked.ID, blocker.ID, Blocks)

	ready, err := store.Ready()
	if err != nil {
		t.Fatalf("ready: %v", err)
	}
	// blocker and free should be ready, blocked should not
	readyIDs := map[string]bool{}
	for _, r := range ready {
		readyIDs[r.ID] = true
	}
	if !readyIDs[blocker.ID] {
		t.Error("blocker should be ready")
	}
	if !readyIDs[free.ID] {
		t.Error("free should be ready")
	}
	if readyIDs[blocked.ID] {
		t.Error("blocked should NOT be ready")
	}

	blockedTasks, err := store.Blocked()
	if err != nil {
		t.Fatalf("blocked: %v", err)
	}
	if len(blockedTasks) != 1 || blockedTasks[0].ID != blocked.ID {
		t.Errorf("expected 1 blocked task, got %d", len(blockedTasks))
	}

	// Close the blocker — now the blocked task should become ready.
	store.CloseTask(blocker.ID, "done", "test")

	ready, _ = store.Ready()
	readyIDs = map[string]bool{}
	for _, r := range ready {
		readyIDs[r.ID] = true
	}
	if !readyIDs[blocked.ID] {
		t.Error("previously blocked task should now be ready")
	}

	blockedTasks, _ = store.Blocked()
	if len(blockedTasks) != 0 {
		t.Error("no tasks should be blocked now")
	}
}
