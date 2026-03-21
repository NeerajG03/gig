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
	if err := s.validateTaskExists(p.ParentID); err != nil {
		return nil, err
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

// GetFull retrieves a task with its custom attributes populated.
func (s *Store) GetFull(id string) (*Task, error) {
	task, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	attrs, err := s.Attrs(id)
	if err != nil {
		return task, nil 
	}
	if len(attrs) > 0 {
		task.Attrs = make(map[string]string, len(attrs))
		for _, a := range attrs {
			task.Attrs[a.Key] = a.Value
		}
	}
	return task, nil
}

func (s *Store) validateTaskExists(taskID string) error {
	if taskID == "" {
		return nil
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ?", taskID).Scan(&count); err != nil {
		return fmt.Errorf("parent task not found: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("parent task not found")
	}
	return nil
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
	if p.Orphan && task.ParentID != "" {
		s.recordEvent(id, EventUpdated, actor, "parent_id", task.ParentID, "")
		sets = append(sets, "parent_id = NULL")
	} else if p.ParentID != nil && *p.ParentID != task.ParentID {
		if *p.ParentID == "" {
			return nil, fmt.Errorf("parent ID cannot be empty, use Orphan to remove parent")
		}
		if *p.ParentID == id {
			return nil, fmt.Errorf("task cannot be its own parent")
		}
		if err := s.validateTaskExists(*p.ParentID); err != nil {
			return nil, err
		}
		s.recordEvent(id, EventUpdated, actor, "parent_id", task.ParentID, *p.ParentID)
		sets = append(sets, "parent_id = ?")
		args = append(args, *p.ParentID)
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
// If the task transitions to in_progress and has a parent that is open,
// the parent is auto-progressed to in_progress.
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

	// Auto-progress parent when child becomes active.
	if status == StatusInProgress {
		s.autoProgressParent(task.ParentID, actor)
	}

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

	// Reject if any child is not closed.
	children, err := s.Children(id)
	if err != nil {
		return fmt.Errorf("check children: %w", err)
	}
	for _, c := range children {
		if c.Status != StatusClosed {
			return fmt.Errorf("cannot close %s: child %s (%s) is %s — close all children first", id, c.ID, c.Title, c.Status)
		}
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

	if err := s.autoUnblock(id, actor); err != nil {
		return fmt.Errorf("auto-unblock after closing %s: %w", id, err)
	}
	return nil
}

// CancelTask sets a task to cancelled with a reason. Also triggers auto-unblock.
// Cancelling a parent cascades to all non-terminal children.
func (s *Store) CancelTask(id string, reason string, actor string) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}
	if task.Status == StatusCancelled {
		return nil
	}

	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET status = ?, closed_at = ?, close_reason = ?, updated_at = ? WHERE id = ?",
		string(StatusCancelled), now.Format(timeFormat), reason, now.Format(timeFormat), id,
	)
	if err != nil {
		return fmt.Errorf("cancel task: %w", err)
	}

	s.recordEvent(id, EventStatusChanged, actor, "status", string(task.Status), string(StatusCancelled))

	if err := s.autoUnblock(id, actor); err != nil {
		return fmt.Errorf("auto-unblock after cancelling %s: %w", id, err)
	}

	// Cascade cancel to all non-terminal children.
	children, err := s.Children(id)
	if err != nil {
		return fmt.Errorf("cascade cancel children: %w", err)
	}
	for _, c := range children {
		if !c.Status.IsTerminal() {
			if err := s.CancelTask(c.ID, "parent cancelled", actor); err != nil {
				return fmt.Errorf("cascade cancel %s: %w", c.ID, err)
			}
		}
	}

	return nil
}

// autoUnblock checks tasks that depend on the given task. If a dependent is
// blocked and all its blockers are now terminal (closed/cancelled), it transitions to open.
func (s *Store) autoUnblock(closedID string, actor string) error {
	dependents, err := s.ListDependents(closedID)
	if err != nil {
		return err
	}

	for _, dep := range dependents {
		if dep.Type != Blocks {
			continue
		}
		task, err := s.Get(dep.FromID)
		if err != nil || task.Status != StatusBlocked {
			continue
		}

		// Check if all blockers for this task are now terminal.
		blockers, err := s.ListDependencies(task.ID)
		if err != nil {
			continue
		}
		allResolved := true
		for _, b := range blockers {
			if b.Type != Blocks {
				continue
			}
			blocker, err := s.Get(b.ToID)
			if err != nil || !blocker.Status.IsTerminal() {
				allResolved = false
				break
			}
		}

		if allResolved {
			now := timeNowUTC()
			_, err := s.db.Exec(
				"UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
				string(StatusOpen), now.Format(timeFormat), task.ID,
			)
			if err != nil {
				return fmt.Errorf("unblock %s: %w", task.ID, err)
			}
			s.recordEvent(task.ID, EventStatusChanged, actor, "status", string(StatusBlocked), string(StatusOpen))
		}
	}
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

// Reopen sets a closed or cancelled task back to open.
func (s *Store) Reopen(id string, actor string) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}
	if !task.Status.IsTerminal() {
		return fmt.Errorf("task %s is not closed or cancelled (status: %s)", id, task.Status)
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

// DeleteTask permanently removes a task and its children from the database.
// Comments, dependencies, and custom attributes are removed via CASCADE.
// Events are preserved as an audit trail.
func (s *Store) DeleteTask(id string, actor string) error {
	task, err := s.Get(id)
	if err != nil {
		return err
	}

	// Recursively delete children first (depth-first).
	children, err := s.Children(id)
	if err != nil {
		return fmt.Errorf("delete task: list children: %w", err)
	}
	for _, child := range children {
		if err := s.DeleteTask(child.ID, actor); err != nil {
			return fmt.Errorf("delete child %s: %w", child.ID, err)
		}
	}

	// Delete events first (no CASCADE on events FK).
	_, err = s.db.Exec("DELETE FROM events WHERE task_id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task events: %w", err)
	}

	_, err = s.db.Exec("DELETE FROM tasks WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete task: %w", err)
	}

	// Emit to listeners only — can't insert event since task row is gone (FK).
	s.emit(Event{
		TaskID:    id,
		Type:      EventDeleted,
		Actor:     actor,
		Field:     "title",
		OldValue:  task.Title,
		Timestamp: timeNowUTC(),
	})
	return nil
}

// ClaimResult holds information about what happened during a Claim.
type ClaimResult struct {
	ParentProgressed bool   // true if parent was auto-moved to in_progress
	ParentID         string // parent task ID (if progressed)
}

// Claim atomically sets assignee and status to in_progress.
// If the task has a parent that is open, it is auto-progressed to in_progress.
func (s *Store) Claim(id string, assignee string) (*ClaimResult, error) {
	task, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	if task.Status == StatusClosed {
		return nil, fmt.Errorf("cannot claim closed task %s", id)
	}

	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET assignee = ?, status = ?, updated_at = ? WHERE id = ?",
		assignee, string(StatusInProgress), now.Format(timeFormat), id,
	)
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}

	if task.Assignee != assignee {
		s.recordEvent(id, EventAssigned, assignee, "assignee", task.Assignee, assignee)
	}
	if task.Status != StatusInProgress {
		s.recordEvent(id, EventStatusChanged, assignee, "status", string(task.Status), string(StatusInProgress))
	}

	result := &ClaimResult{}

	// Auto-progress parent when child becomes active.
	if s.autoProgressParent(task.ParentID, assignee) {
		result.ParentProgressed = true
		result.ParentID = task.ParentID
	}

	return result, nil
}

// autoProgressParent moves a parent task from open to in_progress
// when one of its children becomes active. Returns true if progressed.
func (s *Store) autoProgressParent(parentID, actor string) bool {
	if parentID == "" {
		return false
	}
	parent, err := s.Get(parentID)
	if err != nil || parent.Status != StatusOpen {
		return false
	}
	now := timeNowUTC()
	_, err = s.db.Exec(
		"UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
		string(StatusInProgress), now.Format(timeFormat), parent.ID,
	)
	if err != nil {
		return false
	}
	s.recordEvent(parent.ID, EventStatusChanged, actor, "status", string(StatusOpen), string(StatusInProgress))
	return true
}
