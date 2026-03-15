package gig

import (
	"testing"
	"time"
)

func TestEventsRecorded(t *testing.T) {
	store, _ := tempDB(t)

	task, _ := store.Create(CreateParams{Title: "Task", CreatedBy: "test-actor"})

	events, err := store.Events(task.ID)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least 1 event after create")
	}
	if events[0].Type != EventCreated {
		t.Errorf("event type = %q, want 'created'", events[0].Type)
	}
	if events[0].Actor != "test-actor" {
		t.Errorf("actor = %q, want 'test-actor'", events[0].Actor)
	}
}

func TestEventsForStatusChange(t *testing.T) {
	store, _ := tempDB(t)

	task, _ := store.Create(CreateParams{Title: "Task"})
	store.UpdateStatus(task.ID, StatusInProgress, "agent-1")
	store.CloseTask(task.ID, "done", "agent-1")

	events, _ := store.Events(task.ID)

	// created + status_changed + closed
	statusChanges := 0
	for _, e := range events {
		if e.Type == EventStatusChanged {
			statusChanges++
		}
	}
	if statusChanges < 1 {
		t.Errorf("expected at least 1 status_changed event, got %d", statusChanges)
	}
}

func TestEventsSince(t *testing.T) {
	store, _ := tempDB(t)

	before := time.Now().UTC().Add(-time.Second)
	store.Create(CreateParams{Title: "Task 1"})
	store.Create(CreateParams{Title: "Task 2"})

	events, err := store.EventsSince(before)
	if err != nil {
		t.Fatalf("events since: %v", err)
	}
	if len(events) < 2 {
		t.Errorf("expected at least 2 events, got %d", len(events))
	}
}

func TestEventsSinceFuture(t *testing.T) {
	store, _ := tempDB(t)

	store.Create(CreateParams{Title: "Task"})

	future := time.Now().UTC().Add(time.Hour)
	events, err := store.EventsSince(future)
	if err != nil {
		t.Fatalf("events since: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events in the future, got %d", len(events))
	}
}

func TestEventListenerFires(t *testing.T) {
	store, _ := tempDB(t)

	fired := false
	store.On(EventCreated, func(e Event) {
		fired = true
		if e.Type != EventCreated {
			t.Errorf("event type = %q, want 'created'", e.Type)
		}
	})

	store.Create(CreateParams{Title: "Trigger"})

	if !fired {
		t.Error("expected listener to fire on create")
	}
}

func TestEventListenerOff(t *testing.T) {
	store, _ := tempDB(t)

	count := 0
	store.On(EventCreated, func(e Event) {
		count++
	})

	store.Create(CreateParams{Title: "First"})
	store.Off(EventCreated)
	store.Create(CreateParams{Title: "Second"})

	if count != 1 {
		t.Errorf("expected listener to fire once, fired %d times", count)
	}
}

func TestMultipleListeners(t *testing.T) {
	store, _ := tempDB(t)

	count := 0
	store.On(EventCreated, func(e Event) { count++ })
	store.On(EventCreated, func(e Event) { count++ })

	store.Create(CreateParams{Title: "Task"})

	if count != 2 {
		t.Errorf("expected 2 listener calls, got %d", count)
	}
}

func TestEventsForNonexistentTask(t *testing.T) {
	store, _ := tempDB(t)

	events, err := store.Events("nonexistent")
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events for nonexistent task, got %d", len(events))
	}
}
