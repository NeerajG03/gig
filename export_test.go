package gig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExportImportRoundTrip(t *testing.T) {
	store1, dir := tempDB(t)

	// Create some tasks.
	parent, _ := store1.Create(CreateParams{Title: "Parent", Type: TypeEpic, Priority: P0})
	store1.Create(CreateParams{Title: "Child", ParentID: parent.ID, Priority: P1, Labels: []string{"test"}})
	store1.Create(CreateParams{Title: "Standalone", Priority: P2, Assignee: "neeraj"})

	// Export.
	exportPath := filepath.Join(dir, "tasks.jsonl")
	if err := store1.ExportJSONL(exportPath); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Verify file exists and has content.
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("export file is empty")
	}

	// Import into a fresh store.
	db2Path := filepath.Join(dir, "test2.db")
	store2, err := Open(db2Path, WithPrefix("test"))
	if err != nil {
		t.Fatalf("open store2: %v", err)
	}
	defer store2.Close()

	if err := store2.ImportJSONL(exportPath); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify tasks were imported.
	tasks, _ := store2.List(ListParams{})
	if len(tasks) != 3 {
		t.Errorf("expected 3 imported tasks, got %d", len(tasks))
	}

	// Verify specific task data survived round-trip.
	got, err := store2.Get(parent.ID)
	if err != nil {
		t.Fatalf("get imported parent: %v", err)
	}
	if got.Title != "Parent" || got.Type != TypeEpic {
		t.Errorf("imported task mismatch: title=%q type=%q", got.Title, got.Type)
	}
}

func TestImportUpsert(t *testing.T) {
	store1, dir := tempDB(t)
	task, _ := store1.Create(CreateParams{Title: "Original"})

	// Export.
	exportPath := filepath.Join(dir, "tasks.jsonl")
	if err := store1.ExportJSONL(exportPath); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Update task and re-export.
	newTitle := "Modified"
	store1.Update(task.ID, UpdateParams{Title: &newTitle}, "test")
	store1.ExportJSONL(exportPath)

	// Import into a fresh store — should create.
	db2Path := filepath.Join(dir, "test2.db")
	store2, _ := Open(db2Path, WithPrefix("test"))
	defer store2.Close()

	if err := store2.ImportJSONL(exportPath); err != nil {
		t.Fatalf("import: %v", err)
	}

	got, _ := store2.Get(task.ID)
	if got.Title != "Modified" {
		t.Errorf("expected 'Modified', got %q", got.Title)
	}

	// Import again — should upsert without duplicates.
	if err := store2.ImportJSONL(exportPath); err != nil {
		t.Fatalf("upsert import: %v", err)
	}

	tasks, _ := store2.List(ListParams{})
	if len(tasks) != 1 {
		t.Errorf("expected 1 task after upsert, got %d", len(tasks))
	}
}

func TestExportEvents(t *testing.T) {
	store, dir := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Task"})
	store.UpdateStatus(task.ID, StatusInProgress, "test")
	store.CloseTask(task.ID, "done", "test")

	eventsPath := filepath.Join(dir, "events.jsonl")
	if err := store.ExportEvents(eventsPath); err != nil {
		t.Fatalf("export events: %v", err)
	}

	data, _ := os.ReadFile(eventsPath)
	if len(data) == 0 {
		t.Error("events export is empty")
	}
}
