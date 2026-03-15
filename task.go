package gig

import (
	"database/sql"
	"fmt"
	"strings"
)

// Create inserts a new task and returns it.
func (s *Store) Create(p CreateParams) (*Task, error) {
	if p.Title == "" {
		return nil, fmt.Errorf("title is required")
	}
	if p.Type == "" {
		p.Type = TypeTask
	}
	if !p.Type.IsValid() {
		return nil, fmt.Errorf("invalid task type: %s", p.Type)
	}
	if !p.Priority.IsValid() {
		return nil, fmt.Errorf("invalid priority: %d", p.Priority)
	}

	// Verify parent exists if specified.
	if p.ParentID != "" {
		var exists int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ?", p.ParentID).Scan(&exists); err != nil || exists == 0 {
			return nil, fmt.Errorf("parent task %s not found", p.ParentID)
		}
	}

	now := timeNowUTC()

	// Generate ID: subtasks get ladder notation (parent.1, parent.2, ...)
	var id string
	if p.ParentID != "" {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE parent_id = ?", p.ParentID).Scan(&count); err != nil {
			return nil, fmt.Errorf("count children: %w", err)
		}
		id = fmt.Sprintf("%s.%d", p.ParentID, count+1)
	} else {
		id = s.newID()
	}

	task := &Task{
		ID:          id,
		ParentID:    p.ParentID,
		Title:       p.Title,
		Description: p.Description,
		Status:      StatusOpen,
		Priority:    p.Priority,
		Assignee:    p.Assignee,
		Type:        p.Type,
		Labels:      p.Labels,
		Notes:       p.Notes,
		Estimate:    p.Estimate,
		DueAt:       p.DueAt,
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   p.CreatedBy,
		Metadata:    p.Metadata,
	}

	parentID := sql.NullString{String: p.ParentID, Valid: p.ParentID != ""}

	_, err := s.db.Exec(
		`INSERT INTO tasks (id, parent_id, title, description, status, priority, assignee,
		  task_type, labels, notes, estimate, due_at, created_at, updated_at, created_by, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, parentID, p.Title, p.Description, string(StatusOpen), int(p.Priority), p.Assignee,
		string(p.Type), labelsToJSON(p.Labels), p.Notes, p.Estimate,
		timeToStr(p.DueAt), now.Format(timeFormat), now.Format(timeFormat),
		p.CreatedBy, p.Metadata,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	s.recordEvent(id, EventCreated, p.CreatedBy, "", "", p.Title)
	return task, nil
}

// Get retrieves a single task by ID.
func (s *Store) Get(id string) (*Task, error) {
	return s.scanTask(s.db.QueryRow(
		`SELECT id, parent_id, title, description, status, priority, assignee,
		  task_type, labels, notes, estimate, due_at, created_at, updated_at,
		  closed_at, close_reason, created_by, metadata
		 FROM tasks WHERE id = ?`, id,
	))
}

// Update modifies fields of an existing task.
func (s *Store) Update(id string, p UpdateParams, actor string) (*Task, error) {
	task, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	sets := []string{}
	args := []any{}

	if p.Title != nil && *p.Title != task.Title {
		s.recordEvent(id, EventUpdated, actor, "title", task.Title, *p.Title)
		sets = append(sets, "title = ?")
		args = append(args, *p.Title)
	}
	if p.Description != nil && *p.Description != task.Description {
		s.recordEvent(id, EventUpdated, actor, "description", task.Description, *p.Description)
		sets = append(sets, "description = ?")
		args = append(args, *p.Description)
	}
	if p.Priority != nil && *p.Priority != task.Priority {
		s.recordEvent(id, EventUpdated, actor, "priority", fmt.Sprintf("%d", task.Priority), fmt.Sprintf("%d", *p.Priority))
		sets = append(sets, "priority = ?")
		args = append(args, int(*p.Priority))
	}
	if p.Assignee != nil && *p.Assignee != task.Assignee {
		s.recordEvent(id, EventAssigned, actor, "assignee", task.Assignee, *p.Assignee)
		sets = append(sets, "assignee = ?")
		args = append(args, *p.Assignee)
	}
	if p.Labels != nil {
		s.recordEvent(id, EventUpdated, actor, "labels", labelsToJSON(task.Labels), labelsToJSON(*p.Labels))
		sets = append(sets, "labels = ?")
		args = append(args, labelsToJSON(*p.Labels))
	}
	if p.Notes != nil && *p.Notes != task.Notes {
		s.recordEvent(id, EventUpdated, actor, "notes", task.Notes, *p.Notes)
		sets = append(sets, "notes = ?")
		args = append(args, *p.Notes)
	}
	if p.Estimate != nil && *p.Estimate != task.Estimate {
		s.recordEvent(id, EventUpdated, actor, "estimate", fmt.Sprintf("%d", task.Estimate), fmt.Sprintf("%d", *p.Estimate))
		sets = append(sets, "estimate = ?")
		args = append(args, *p.Estimate)
	}
	if p.DueAt != nil {
		sets = append(sets, "due_at = ?")
		args = append(args, timeToStr(p.DueAt))
	}
	if p.Metadata != nil && *p.Metadata != task.Metadata {
		sets = append(sets, "metadata = ?")
		args = append(args, *p.Metadata)
	}

	if len(sets) == 0 {
		return task, nil // nothing to update
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, timeNowUTC().Format(timeFormat))
	args = append(args, id)

	query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", strings.Join(sets, ", "))
	if _, err := s.db.Exec(query, args...); err != nil {
		return nil, fmt.Errorf("update task: %w", err)
	}

	return s.Get(id)
}

// UpdateStatus changes a task's status.
func (s *Store) UpdateStatus(id string, status Status, actor string) error {
	if !status.IsValid() {
		return fmt.Errorf("invalid status: %s", status)
	}

	task, err := s.Get(id)
	if err != nil {
		return err
	}

	if task.Status == status {
		return nil
	}

	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
		string(status), now.Format(timeFormat), id,
	)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}

	s.recordEvent(id, EventStatusChanged, actor, "status", string(task.Status), string(status))
	return nil
}

// CloseTask marks a task as closed.
func (s *Store) CloseTask(id string, reason string, actor string) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}
	if task.Status == StatusClosed {
		return nil
	}

	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET status = ?, closed_at = ?, close_reason = ?, updated_at = ? WHERE id = ?",
		string(StatusClosed), now.Format(timeFormat), reason, now.Format(timeFormat), id,
	)
	if err != nil {
		return fmt.Errorf("close task: %w", err)
	}

	s.recordEvent(id, EventClosed, actor, "status", string(task.Status), string(StatusClosed))
	return nil
}

// CloseMany closes multiple tasks at once.
func (s *Store) CloseMany(ids []string, reason string, actor string) error {
	for _, id := range ids {
		if err := s.CloseTask(id, reason, actor); err != nil {
			return fmt.Errorf("close %s: %w", id, err)
		}
	}
	return nil
}

// Reopen sets a closed task back to open.
func (s *Store) Reopen(id string, actor string) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}
	if task.Status != StatusClosed {
		return fmt.Errorf("task %s is not closed (status: %s)", id, task.Status)
	}

	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET status = ?, closed_at = '', close_reason = '', updated_at = ? WHERE id = ?",
		string(StatusOpen), now.Format(timeFormat), id,
	)
	if err != nil {
		return fmt.Errorf("reopen task: %w", err)
	}

	s.recordEvent(id, EventStatusChanged, actor, "status", string(StatusClosed), string(StatusOpen))
	return nil
}

// Claim atomically sets assignee and status to in_progress.
func (s *Store) Claim(id string, assignee string) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}
	if task.Status == StatusClosed {
		return fmt.Errorf("cannot claim closed task %s", id)
	}

	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET assignee = ?, status = ?, updated_at = ? WHERE id = ?",
		assignee, string(StatusInProgress), now.Format(timeFormat), id,
	)
	if err != nil {
		return fmt.Errorf("claim task: %w", err)
	}

	if task.Assignee != assignee {
		s.recordEvent(id, EventAssigned, assignee, "assignee", task.Assignee, assignee)
	}
	if task.Status != StatusInProgress {
		s.recordEvent(id, EventStatusChanged, assignee, "status", string(task.Status), string(StatusInProgress))
	}
	return nil
}
