package gig

import (
	"testing"
)

func TestHookMatchesFilter_NoFilter(t *testing.T) {
	h := HookDef{Command: "echo hi"}
	e := Event{TaskID: "t-1", Type: EventStatusChanged, NewValue: "closed"}

	if !hookMatchesFilter(h, e) {
		t.Error("hook with no filter should match any event")
	}
}

func TestHookMatchesFilter_NewStatus(t *testing.T) {
	h := HookDef{
		Command: "echo closed",
		Filter:  map[string]string{"new_status": "closed"},
	}

	match := Event{NewValue: "closed"}
	if !hookMatchesFilter(h, match) {
		t.Error("should match when new_status equals filter")
	}

	noMatch := Event{NewValue: "open"}
	if hookMatchesFilter(h, noMatch) {
		t.Error("should not match when new_status differs")
	}
}

func TestHookMatchesFilter_OldStatus(t *testing.T) {
	h := HookDef{
		Command: "echo was_open",
		Filter:  map[string]string{"old_status": "open"},
	}

	match := Event{OldValue: "open"}
	if !hookMatchesFilter(h, match) {
		t.Error("should match when old_status equals filter")
	}

	noMatch := Event{OldValue: "in_progress"}
	if hookMatchesFilter(h, noMatch) {
		t.Error("should not match when old_status differs")
	}
}

func TestHookMatchesFilter_Assignee(t *testing.T) {
	h := HookDef{
		Command: "echo assigned",
		Filter:  map[string]string{"assignee": "neeraj"},
	}

	match := Event{Actor: "neeraj"}
	if !hookMatchesFilter(h, match) {
		t.Error("should match when actor equals assignee filter")
	}

	noMatch := Event{Actor: "jeff"}
	if hookMatchesFilter(h, noMatch) {
		t.Error("should not match when actor differs from assignee filter")
	}
}

func TestHookMatchesFilter_MultipleConditions(t *testing.T) {
	h := HookDef{
		Command: "echo transition",
		Filter: map[string]string{
			"old_status": "open",
			"new_status": "closed",
		},
	}

	match := Event{OldValue: "open", NewValue: "closed"}
	if !hookMatchesFilter(h, match) {
		t.Error("should match when all conditions met")
	}

	partial := Event{OldValue: "in_progress", NewValue: "closed"}
	if hookMatchesFilter(h, partial) {
		t.Error("should not match when only some conditions met")
	}
}

func TestExpandHookVars(t *testing.T) {
	e := Event{
		TaskID:   "test-abc",
		OldValue: "open",
		NewValue: "closed",
		Actor:    "agent-1",
		Field:    "status",
	}

	cmd := expandHookVars("notify {id} changed {field} from {old} to {new} by {actor}", e)
	expected := "notify test-abc changed status from open to closed by agent-1"
	if cmd != expected {
		t.Errorf("got %q, want %q", cmd, expected)
	}
}

func TestExpandHookVars_NoVars(t *testing.T) {
	cmd := expandHookVars("echo hello", Event{})
	if cmd != "echo hello" {
		t.Errorf("got %q, want %q", cmd, "echo hello")
	}
}

func TestRunHooks_NoConfig(t *testing.T) {
	store, _ := tempDB(t)
	// Should not panic with nil config.
	store.RunHooks(Event{Type: EventCreated})
}

func TestRunHooks_SelectsCorrectHookList(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Prefix:  "test",
		HashLen: 4,
		Hooks: HookConfig{
			OnCreate: []HookDef{{Command: "echo created"}},
			OnClose:  []HookDef{{Command: "echo closed"}},
		},
	}
	store, err := Open(dir+"/test.db", WithPrefix("test"), WithConfig(cfg))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	// This just verifies RunHooks doesn't panic for each event type.
	// Actual execution is fire-and-forget in goroutines.
	store.RunHooks(Event{Type: EventCreated, TaskID: "t-1"})
	store.RunHooks(Event{Type: EventStatusChanged, TaskID: "t-1"})
	store.RunHooks(Event{Type: EventCommented, TaskID: "t-1"})
	store.RunHooks(Event{Type: EventClosed, TaskID: "t-1"})
	store.RunHooks(Event{Type: EventAssigned, TaskID: "t-1"})
}

func TestEnableHooks(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Prefix:  "test",
		HashLen: 4,
		Hooks: HookConfig{
			OnCreate: []HookDef{{Command: "true"}},
		},
	}
	store, err := Open(dir+"/test.db", WithPrefix("test"), WithConfig(cfg))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	store.EnableHooks()

	// Verify listeners were registered.
	store.mu.RLock()
	count := 0
	for _, listeners := range store.listeners {
		count += len(listeners)
	}
	store.mu.RUnlock()

	if count == 0 {
		t.Error("expected listeners to be registered after EnableHooks")
	}
}

func TestEnableHooks_NilConfig(t *testing.T) {
	store, _ := tempDB(t)
	// Should not panic.
	store.EnableHooks()
}
