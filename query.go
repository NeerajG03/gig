package gig

import (
	"database/sql"
	"fmt"
	"strings"
)

// List returns tasks matching the given filters.
func (s *Store) List(p ListParams) ([]*Task, error) {
	where := []string{}
	args := []any{}

	if p.Status != nil {
		where = append(where, "status = ?")
		args = append(args, string(*p.Status))
	}
	if p.Assignee != "" {
		where = append(where, "assignee = ?")
		args = append(args, p.Assignee)
	}
	if p.Priority != nil {
		where = append(where, "priority = ?")
		args = append(args, int(*p.Priority))
	}
	if p.ParentID != nil {
		if *p.ParentID == "" {
			where = append(where, "(parent_id IS NULL OR parent_id = '')")
		} else {
			where = append(where, "parent_id = ?")
			args = append(args, *p.ParentID)
		}
	}
	if p.Type != nil {
		where = append(where, "task_type = ?")
		args = append(args, string(*p.Type))
	}
	if p.Label != "" {
		where = append(where, "labels LIKE ?")
		args = append(args, fmt.Sprintf("%%%s%%", p.Label))
	}
	for k, v := range p.AttrFilter {
		where = append(where, "id IN (SELECT task_id FROM custom_attributes WHERE key = ? AND value = ?)")
		args = append(args, k, v)
	}
	if len(p.ExcludeStatuses) > 0 {
		placeholders := make([]string, len(p.ExcludeStatuses))
		for i, s := range p.ExcludeStatuses {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		where = append(where, "status NOT IN ("+strings.Join(placeholders, ",")+")")
	}

	query := `SELECT id, parent_id, title, description, status, priority, assignee,
	  task_type, labels, notes, estimate, due_at, created_at, updated_at,
	  closed_at, close_reason, created_by, metadata FROM tasks`

	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY priority ASC, updated_at DESC"

	if p.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", p.Limit)
		if p.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", p.Offset)
		}
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	return s.scanTasks(rows)
}

// Search performs a simple text search on title and description.
func (s *Store) Search(query string) ([]*Task, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.Query(
		`SELECT id, parent_id, title, description, status, priority, assignee,
		  task_type, labels, notes, estimate, due_at, created_at, updated_at,
		  closed_at, close_reason, created_by, metadata
		 FROM tasks WHERE title LIKE ? OR description LIKE ?
		 ORDER BY updated_at DESC`,
		pattern, pattern,
	)
	if err != nil {
		return nil, fmt.Errorf("search tasks: %w", err)
	}
	defer rows.Close()

	return s.scanTasks(rows)
}

// Children returns the direct children of a task.
func (s *Store) Children(id string) ([]*Task, error) {
	rows, err := s.db.Query(
		`SELECT id, parent_id, title, description, status, priority, assignee,
		  task_type, labels, notes, estimate, due_at, created_at, updated_at,
		  closed_at, close_reason, created_by, metadata
		 FROM tasks WHERE parent_id = ?
		 ORDER BY priority ASC, created_at ASC`, id,
	)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	return s.scanTasks(rows)
}

// GetTree returns a task with all its descendants populated in Children.
func (s *Store) GetTree(id string) (*Task, error) {
	task, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	children, err := s.Children(id)
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		subtree, err := s.GetTree(child.ID)
		if err != nil {
			return nil, err
		}
		task.Children = append(task.Children, subtree)
	}

	return task, nil
}

// Ready returns open/in_progress tasks that have no unresolved blockers.
// Ready returns open tasks that have no unresolved blockers — i.e. tasks
// available to be picked up. If parentID is non-empty, only returns children
// (direct and nested) of that task.
func (s *Store) Ready(parentID string) ([]*Task, error) {
	query := `SELECT t.id, t.parent_id, t.title, t.description, t.status, t.priority,
		  t.assignee, t.task_type, t.labels, t.notes, t.estimate, t.due_at,
		  t.created_at, t.updated_at, t.closed_at, t.close_reason, t.created_by, t.metadata
		 FROM tasks t
		 WHERE t.status = 'open'
		   AND NOT EXISTS (
		     SELECT 1 FROM dependencies d
		     JOIN tasks blocker ON blocker.id = d.to_id
		     WHERE d.from_id = t.id
		       AND d.dep_type = 'blocks'
		       AND blocker.status NOT IN ('closed', 'cancelled')
		   )`

	var args []any
	if parentID != "" {
		// Match direct children and nested descendants (e.g. parent.1, parent.1.2).
		query += ` AND (t.parent_id = ? OR t.id LIKE ?)`
		args = append(args, parentID, parentID+".%")
	}

	query += ` ORDER BY t.priority ASC, t.updated_at DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ready tasks: %w", err)
	}
	defer rows.Close()

	return s.scanTasks(rows)
}

// Blocked returns tasks that have at least one unresolved blocker.
func (s *Store) Blocked() ([]*Task, error) {
	rows, err := s.db.Query(
		`SELECT t.id, t.parent_id, t.title, t.description, t.status, t.priority,
		  t.assignee, t.task_type, t.labels, t.notes, t.estimate, t.due_at,
		  t.created_at, t.updated_at, t.closed_at, t.close_reason, t.created_by, t.metadata
		 FROM tasks t
		 WHERE t.status NOT IN ('closed', 'cancelled')
		   AND EXISTS (
		     SELECT 1 FROM dependencies d
		     JOIN tasks blocker ON blocker.id = d.to_id
		     WHERE d.from_id = t.id
		       AND d.dep_type = 'blocks'
		       AND blocker.status NOT IN ('closed', 'cancelled')
		   )
		 ORDER BY t.priority ASC, t.updated_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("blocked tasks: %w", err)
	}
	defer rows.Close()

	return s.scanTasks(rows)
}

// scanTask reads a single task from a row scanner.
func (s *Store) scanTask(row *sql.Row) (*Task, error) {
	var t Task
	var parentID sql.NullString
	var labelsJSON, dueAt, closedAt, createdAt, updatedAt string

	err := row.Scan(
		&t.ID, &parentID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.Assignee, &t.Type, &labelsJSON, &t.Notes, &t.Estimate, &dueAt,
		&createdAt, &updatedAt, &closedAt, &t.CloseReason, &t.CreatedBy, &t.Metadata,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("task not found")
		}
		return nil, fmt.Errorf("scan task: %w", err)
	}

	if parentID.Valid {
		t.ParentID = parentID.String
	}
	t.Labels = labelsFromJSON(labelsJSON)
	t.DueAt = strToTime(dueAt)
	t.ClosedAt = strToTime(closedAt)
	if ct := strToTime(createdAt); ct != nil {
		t.CreatedAt = *ct
	}
	if ut := strToTime(updatedAt); ut != nil {
		t.UpdatedAt = *ut
	}

	return &t, nil
}

// scanTasks reads multiple tasks from rows.
func (s *Store) scanTasks(rows *sql.Rows) ([]*Task, error) {
	var tasks []*Task
	for rows.Next() {
		var t Task
		var parentID sql.NullString
		var labelsJSON, dueAt, closedAt, createdAt, updatedAt string

		err := rows.Scan(
			&t.ID, &parentID, &t.Title, &t.Description, &t.Status, &t.Priority,
			&t.Assignee, &t.Type, &labelsJSON, &t.Notes, &t.Estimate, &dueAt,
			&createdAt, &updatedAt, &closedAt, &t.CloseReason, &t.CreatedBy, &t.Metadata,
		)
		if err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}

		if parentID.Valid {
			t.ParentID = parentID.String
		}
		t.Labels = labelsFromJSON(labelsJSON)
		t.DueAt = strToTime(dueAt)
		t.ClosedAt = strToTime(closedAt)
		if ct := strToTime(createdAt); ct != nil {
			t.CreatedAt = *ct
		}
		if ut := strToTime(updatedAt); ut != nil {
			t.UpdatedAt = *ut
		}

		tasks = append(tasks, &t)
	}

	return tasks, rows.Err()
}
