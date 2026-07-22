package gig

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestCreateCollisionRetry drives the ID space small (WithHashLength(3) = 4096
// values) and creates enough tasks that the birthday bound forces collisions.
// Every Create must still succeed — the retry loop regenerates on a UNIQUE
// violation rather than surfacing a raw constraint error (finding #4).
func TestCreateCollisionRetry(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(filepath.Join(dir, "test.db"), WithPrefix("test"), WithHashLength(3))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer store.Close()

	seen := make(map[string]bool)
	for i := 0; i < 300; i++ {
		task, err := store.Create(CreateParams{Title: fmt.Sprintf("t%d", i)})
		if err != nil {
			t.Fatalf("create %d failed (retry loop should have recovered): %v", i, err)
		}
		if seen[task.ID] {
			t.Fatalf("duplicate ID minted: %s", task.ID)
		}
		seen[task.ID] = true
	}
}

// TestSubtaskLadderRace creates many children of one parent concurrently and
// asserts every child gets a unique ladder ID — the COUNT+INSERT now shares a
// transaction and retries on collision (finding #4, ladder race).
func TestSubtaskLadderRace(t *testing.T) {
	store, _ := tempDB(t)
	parent, _ := store.Create(CreateParams{Title: "Parent", Type: TypeEpic})

	const n = 40
	var wg sync.WaitGroup
	ids := make(chan string, n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			child, err := store.Create(CreateParams{Title: fmt.Sprintf("c%d", i), ParentID: parent.ID})
			if err != nil {
				errs <- err
				return
			}
			ids <- child.ID
		}(i)
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		t.Errorf("child create failed: %v", err)
	}
	seen := make(map[string]bool)
	count := 0
	for id := range ids {
		if seen[id] {
			t.Errorf("duplicate ladder ID: %s", id)
		}
		seen[id] = true
		count++
	}
	if count != n {
		t.Errorf("expected %d children, got %d", n, count)
	}
}

// TestEventRollbackOnFailedInsert drops the events table so the event INSERT
// inside a mutation's transaction fails; the mutation itself must roll back
// (finding #3 — write + audit are atomic).
func TestEventRollbackOnFailedInsert(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Task"})

	if _, err := store.DB().Exec("DROP TABLE events"); err != nil {
		t.Fatalf("drop events: %v", err)
	}

	// UpdateStatus should now fail because its event insert fails.
	err := store.UpdateStatus(task.ID, StatusInProgress, "actor")
	if err == nil {
		t.Fatal("expected UpdateStatus to fail after events table dropped")
	}

	// Recreate events so we can read task state, then confirm status unchanged.
	if _, rerr := store.DB().Exec(`CREATE TABLE events (
		id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT NOT NULL, event_type TEXT NOT NULL,
		actor TEXT DEFAULT '', field TEXT DEFAULT '', old_value TEXT DEFAULT '',
		new_value TEXT DEFAULT '', timestamp TEXT NOT NULL)`); rerr != nil {
		t.Fatalf("recreate events: %v", rerr)
	}
	got, _ := store.Get(task.ID)
	if got.Status != StatusOpen {
		t.Errorf("status should have rolled back to open, got %q", got.Status)
	}
}

// TestEventsAfterID verifies the integer-cursor pagination and ordering.
func TestEventsAfterID(t *testing.T) {
	store, _ := tempDB(t)

	// Generate a known sequence of events.
	task, _ := store.Create(CreateParams{Title: "Task"}) // EventCreated (id 1)
	store.UpdateStatus(task.ID, StatusInProgress, "a")    // EventStatusChanged (id 2)
	store.UpdateStatus(task.ID, StatusBlocked, "a")       // id 3
	store.UpdateStatus(task.ID, StatusOpen, "a")          // id 4

	all, err := store.EventsAfterID(0, 0)
	if err != nil {
		t.Fatalf("EventsAfterID: %v", err)
	}
	if len(all) < 4 {
		t.Fatalf("expected >=4 events, got %d", len(all))
	}

	// Strictly increasing ids.
	for i := 1; i < len(all); i++ {
		if all[i].ID <= all[i-1].ID {
			t.Errorf("ids not strictly increasing: %d then %d", all[i-1].ID, all[i].ID)
		}
	}

	// Pagination: limit 2 from the start, then resume from the last id.
	page1, _ := store.EventsAfterID(0, 2)
	if len(page1) != 2 {
		t.Fatalf("page1 expected 2, got %d", len(page1))
	}
	page2, _ := store.EventsAfterID(page1[1].ID, 2)
	if len(page2) < 1 {
		t.Fatalf("page2 expected >=1, got %d", len(page2))
	}
	if page2[0].ID <= page1[1].ID {
		t.Errorf("resume cursor leaked/duplicated: page2 starts at %d, cursor was %d", page2[0].ID, page1[1].ID)
	}

	// afterID past the end returns empty.
	last := all[len(all)-1].ID
	empty, _ := store.EventsAfterID(last, 10)
	if len(empty) != 0 {
		t.Errorf("expected no events after last id, got %d", len(empty))
	}
}

// TestDefaultHashLength confirms the default ID length bumped 4 -> 6.
func TestDefaultHashLength(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Task"})
	// prefix "test" + "-" + 6 hex chars.
	parts := strings.SplitN(task.ID, "-", 2)
	if len(parts) != 2 || len(parts[1]) != 6 {
		t.Errorf("expected 6-char hash, got ID %q", task.ID)
	}
}
