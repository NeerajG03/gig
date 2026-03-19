package gig

import "testing"

func TestAddCheckpoint(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test task"})

	cp, err := store.AddCheckpoint(task.ID, "agent", CheckpointParams{
		Done:      "Implemented auth flow",
		Decisions: "Chose JWT over sessions for statelessness",
		Next:      "Add refresh token support",
		Blockers:  "Waiting on key rotation config",
		Files:     []string{"auth/handler.go", "auth/token.go"},
	})
	if err != nil {
		t.Fatalf("add checkpoint: %v", err)
	}
	if cp.Done != "Implemented auth flow" {
		t.Errorf("expected done text, got %q", cp.Done)
	}
	if len(cp.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(cp.Files))
	}
}

func TestAddCheckpointRequiresDone(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test task"})

	_, err := store.AddCheckpoint(task.ID, "agent", CheckpointParams{})
	if err == nil {
		t.Error("expected error for empty done field")
	}
}

func TestAddCheckpointInvalidTask(t *testing.T) {
	store, _ := tempDB(t)

	_, err := store.AddCheckpoint("nonexistent", "agent", CheckpointParams{Done: "stuff"})
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestListCheckpoints(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test task"})

	store.AddCheckpoint(task.ID, "agent", CheckpointParams{
		Done: "First pass",
		Files: []string{"a.go"},
	})
	store.AddCheckpoint(task.ID, "agent", CheckpointParams{
		Done: "Second pass",
		Next: "Review needed",
		Files: []string{"b.go", "c.go"},
	})

	cps, err := store.ListCheckpoints(task.ID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(cps) != 2 {
		t.Fatalf("expected 2 checkpoints, got %d", len(cps))
	}
	if cps[0].Done != "First pass" {
		t.Errorf("expected first checkpoint done, got %q", cps[0].Done)
	}
	if len(cps[0].Files) != 1 {
		t.Errorf("expected 1 file on first checkpoint, got %d", len(cps[0].Files))
	}
	if len(cps[1].Files) != 2 {
		t.Errorf("expected 2 files on second checkpoint, got %d", len(cps[1].Files))
	}
	if cps[1].Next != "Review needed" {
		t.Errorf("expected next text, got %q", cps[1].Next)
	}
}

func TestListCheckpointsEmpty(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "No checkpoints"})

	cps, err := store.ListCheckpoints(task.ID)
	if err != nil {
		t.Fatalf("list checkpoints: %v", err)
	}
	if len(cps) != 0 {
		t.Errorf("expected 0 checkpoints, got %d", len(cps))
	}
}

func TestLatestCheckpoint(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test task"})

	// No checkpoints yet.
	cp, err := store.LatestCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("latest checkpoint: %v", err)
	}
	if cp != nil {
		t.Error("expected nil for no checkpoints")
	}

	store.AddCheckpoint(task.ID, "agent", CheckpointParams{Done: "First"})
	store.AddCheckpoint(task.ID, "agent", CheckpointParams{Done: "Second", Files: []string{"x.go"}})

	cp, err = store.LatestCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("latest checkpoint: %v", err)
	}
	if cp == nil {
		t.Fatal("expected a checkpoint")
	}
	if cp.Done != "Second" {
		t.Errorf("expected latest done, got %q", cp.Done)
	}
	if len(cp.Files) != 1 || cp.Files[0] != "x.go" {
		t.Errorf("expected files [x.go], got %v", cp.Files)
	}
}

func TestCheckpointNoFiles(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test task"})

	cp, err := store.AddCheckpoint(task.ID, "agent", CheckpointParams{
		Done: "Refactored module",
	})
	if err != nil {
		t.Fatalf("add checkpoint: %v", err)
	}
	if len(cp.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(cp.Files))
	}

	// Verify round-trip via list.
	cps, _ := store.ListCheckpoints(task.ID)
	if len(cps) != 1 || len(cps[0].Files) != 0 {
		t.Error("expected checkpoint with no files after round-trip")
	}
}
