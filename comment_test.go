package gig

import (
	"testing"
	"time"
)

func TestAddAndListComments(t *testing.T) {
	store, _ := tempDB(t)

	task, err := store.Create(CreateParams{Title: "Comment test"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	c1, err := store.AddComment(task.ID, "alice", "First comment")
	if err != nil {
		t.Fatalf("add comment 1: %v", err)
	}
	if c1.Author != "alice" || c1.Content != "First comment" {
		t.Errorf("unexpected comment: %+v", c1)
	}

	_, err = store.AddComment(task.ID, "bob", "Second comment")
	if err != nil {
		t.Fatalf("add comment 2: %v", err)
	}

	comments, err := store.ListComments(task.ID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}

	// Verify ordering (oldest first).
	if comments[0].Content != "First comment" {
		t.Errorf("expected first comment first, got %q", comments[0].Content)
	}
	if comments[1].Content != "Second comment" {
		t.Errorf("expected second comment second, got %q", comments[1].Content)
	}
}

func TestListCommentsCreatedAtParsed(t *testing.T) {
	store, _ := tempDB(t)

	task, err := store.Create(CreateParams{Title: "Time parse test"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	before := time.Now().UTC().Add(-time.Second)
	_, err = store.AddComment(task.ID, "tester", "Check time parsing")
	if err != nil {
		t.Fatalf("add comment: %v", err)
	}

	comments, err := store.ListComments(task.ID)
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(comments))
	}

	// CreatedAt must be parsed from the RFC3339 string stored in SQLite,
	// not zero. This is a regression test — scanning time.Time directly
	// from SQLite fails because the driver returns a string, not time.Time.
	c := comments[0]
	if c.CreatedAt.IsZero() {
		t.Fatal("CreatedAt is zero — time was not parsed from SQLite string")
	}
	if c.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before test start %v", c.CreatedAt, before)
	}
}

func TestAddCommentEmptyContent(t *testing.T) {
	store, _ := tempDB(t)

	task, err := store.Create(CreateParams{Title: "Empty comment test"})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}

	_, err = store.AddComment(task.ID, "alice", "")
	if err == nil {
		t.Error("expected error for empty comment content")
	}
}

func TestAddCommentNonexistentTask(t *testing.T) {
	store, _ := tempDB(t)

	_, err := store.AddComment("nonexistent-id", "alice", "Hello")
	if err == nil {
		t.Error("expected error for comment on nonexistent task")
	}
}
