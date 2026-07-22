package gig

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestConcurrentWriteHammer fires 8 goroutines x 50 mixed Create/UpdateStatus
// operations at a single store and asserts zero "database is locked" errors.
// This exercises finding #1: without DSN _pragma busy_timeout + SetMaxOpenConns(1),
// pooled connections default to busy_timeout=0 and fail instantly under contention.
func TestConcurrentWriteHammer(t *testing.T) {
	store, _ := tempDB(t)

	const workers = 8
	const ops = 50

	var wg sync.WaitGroup
	errCh := make(chan error, workers*ops)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < ops; i++ {
				task, err := store.Create(CreateParams{
					Title: fmt.Sprintf("w%d-task%d", w, i),
				})
				if err != nil {
					errCh <- fmt.Errorf("create: %w", err)
					continue
				}
				if err := store.UpdateStatus(task.ID, StatusInProgress, "hammer"); err != nil {
					errCh <- fmt.Errorf("update: %w", err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errCh)

	var locked, other int
	for err := range errCh {
		if strings.Contains(err.Error(), "database is locked") {
			locked++
		} else {
			other++
			t.Errorf("unexpected error: %v", err)
		}
	}
	if locked > 0 {
		t.Errorf("got %d 'database is locked' errors — busy_timeout/SetMaxOpenConns not applied", locked)
	}
}
