package gig

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// TestClaimConcurrent verifies the compare-and-swap: two goroutines racing to
// claim the same open task result in exactly one winner; the loser gets
// ErrAlreadyClaimed (finding #2 — previously both silently "won").
func TestClaimConcurrent(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Contended"})

	var wins, claimed int64
	var otherErr error
	var wg sync.WaitGroup
	start := make(chan struct{})

	for _, who := range []string{"agent-a", "agent-b"} {
		wg.Add(1)
		go func(assignee string) {
			defer wg.Done()
			<-start
			_, err := store.Claim(task.ID, assignee)
			switch {
			case err == nil:
				atomic.AddInt64(&wins, 1)
			case errors.Is(err, ErrAlreadyClaimed):
				atomic.AddInt64(&claimed, 1)
			default:
				otherErr = err
			}
		}(who)
	}
	close(start)
	wg.Wait()

	if otherErr != nil {
		t.Fatalf("unexpected error: %v", otherErr)
	}
	if wins != 1 {
		t.Errorf("expected exactly 1 winner, got %d", wins)
	}
	if claimed != 1 {
		t.Errorf("expected exactly 1 ErrAlreadyClaimed, got %d", claimed)
	}
}

// TestClaimSameAssigneeReclaim verifies idempotent resume: the same assignee
// re-claiming an in_progress task succeeds.
func TestClaimSameAssigneeReclaim(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Resumable"})

	if _, err := store.Claim(task.ID, "agent-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if _, err := store.Claim(task.ID, "agent-1"); err != nil {
		t.Fatalf("same-assignee re-claim should succeed, got: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.Assignee != "agent-1" || got.Status != StatusInProgress {
		t.Errorf("after re-claim: assignee=%q status=%q", got.Assignee, got.Status)
	}
}

// TestClaimDifferentAssigneeFails verifies a different assignee claiming an
// already-claimed task gets ErrAlreadyClaimed and the original claim stands.
func TestClaimDifferentAssigneeFails(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Held"})

	if _, err := store.Claim(task.ID, "agent-1"); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	_, err := store.Claim(task.ID, "agent-2")
	if !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed, got: %v", err)
	}
	got, _ := store.Get(task.ID)
	if got.Assignee != "agent-1" {
		t.Errorf("assignee should still be agent-1, got %q", got.Assignee)
	}
}

// TestClaimTerminalFails verifies terminal tasks cannot be claimed.
func TestClaimTerminalFails(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Done"})
	if err := store.CloseTask(task.ID, "finished", "actor"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := store.Claim(task.ID, "agent-1"); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed for closed task, got: %v", err)
	}
}

// TestClaimBlockedAndDeferred verifies non-terminal, non-in_progress statuses
// (blocked, deferred) remain claimable — preserving prior behaviour.
func TestClaimBlockedAndDeferred(t *testing.T) {
	for _, st := range []Status{StatusBlocked, StatusDeferred} {
		store, _ := tempDB(t)
		task, _ := store.Create(CreateParams{Title: "Task"})
		if err := store.UpdateStatus(task.ID, st, "actor"); err != nil {
			t.Fatalf("set %s: %v", st, err)
		}
		if _, err := store.Claim(task.ID, "agent-1"); err != nil {
			t.Errorf("claiming %s task should succeed, got: %v", st, err)
		}
	}
}
