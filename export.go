package gig

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// ExportJSONL exports all tasks to a JSONL file, sorted deterministically by ID.
func (s *Store) ExportJSONL(path string) error {
	tasks, err := s.List(ListParams{})
	if err != nil {
		return fmt.Errorf("list tasks for export: %w", err)
	}

	// Deterministic sort by ID for clean git diffs.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, t := range tasks {
		if err := enc.Encode(t); err != nil {
			return fmt.Errorf("encode task %s: %w", t.ID, err)
		}
	}

	return nil
}

// ImportJSONL imports tasks from a JSONL file using upsert semantics.
// Existing tasks are updated, new tasks are inserted.
// Foreign key checks are deferred during import to handle parent ordering.
func (s *Store) ImportJSONL(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open import file: %w", err)
	}
	defer f.Close()

	// Read all tasks first.
	var tasks []Task
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var task Task
		if err := json.Unmarshal(line, &task); err != nil {
			return fmt.Errorf("decode task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	// Sort: root tasks first (no parent), then children.
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].ParentID == "" && tasks[j].ParentID != "" {
			return true
		}
		return false
	})

	// Temporarily disable FK checks for import.
	if _, err := s.db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer s.db.Exec("PRAGMA foreign_keys=ON")

	for _, task := range tasks {
		if err := s.upsertTask(&task); err != nil {
			return fmt.Errorf("upsert task %s: %w", task.ID, err)
		}
	}

	return nil
}

// ExportEvents exports all events to a JSONL file.
func (s *Store) ExportEvents(path string) error {
	rows, err := s.db.Query(
		`SELECT id, task_id, event_type, actor, field, old_value, new_value, timestamp
		 FROM events ORDER BY id ASC`,
	)
	if err != nil {
		return fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	events, err := s.scanEvents(rows)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create events file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	for _, e := range events {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("encode event: %w", err)
		}
	}

	return nil
}

// upsertTask inserts or updates a task from imported data.
func (s *Store) upsertTask(t *Task) error {
	_, err := s.db.Exec(
		`INSERT INTO tasks (id, parent_id, title, description, status, priority, assignee,
		  task_type, labels, notes, estimate, due_at, created_at, updated_at,
		  closed_at, close_reason, created_by, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		  parent_id = excluded.parent_id,
		  title = excluded.title,
		  description = excluded.description,
		  status = excluded.status,
		  priority = excluded.priority,
		  assignee = excluded.assignee,
		  task_type = excluded.task_type,
		  labels = excluded.labels,
		  notes = excluded.notes,
		  estimate = excluded.estimate,
		  due_at = excluded.due_at,
		  updated_at = excluded.updated_at,
		  closed_at = excluded.closed_at,
		  close_reason = excluded.close_reason,
		  metadata = excluded.metadata`,
		t.ID, t.ParentID, t.Title, t.Description, string(t.Status), int(t.Priority),
		t.Assignee, string(t.Type), labelsToJSON(t.Labels), t.Notes, t.Estimate,
		timeToStr(t.DueAt), t.CreatedAt.Format(timeFormat), t.UpdatedAt.Format(timeFormat),
		timeToStr(t.ClosedAt), t.CloseReason, t.CreatedBy, t.Metadata,
	)
	return err
}
