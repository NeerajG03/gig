package gig

import "fmt"

// AddComment creates a comment on a task.
func (s *Store) AddComment(taskID, author, content string) (*Comment, error) {
	if content == "" {
		return nil, fmt.Errorf("comment content is required")
	}

	// Verify task exists.
	if _, err := s.Get(taskID); err != nil {
		return nil, err
	}

	id := s.newID()
	now := timeNowUTC()

	c := &Comment{
		ID:        id,
		TaskID:    taskID,
		Author:    author,
		Content:   content,
		CreatedAt: now,
	}

	_, err := s.db.Exec(
		"INSERT INTO comments (id, task_id, author, content, created_at) VALUES (?, ?, ?, ?, ?)",
		id, taskID, author, content, now.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("insert comment: %w", err)
	}

	s.recordEvent(taskID, EventCommented, author, "", "", content)
	return c, nil
}

// ListComments returns all comments for a task, oldest first.
func (s *Store) ListComments(taskID string) ([]*Comment, error) {
	rows, err := s.db.Query(
		"SELECT id, task_id, author, content, created_at FROM comments WHERE task_id = ? ORDER BY created_at ASC",
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.Author, &c.Content, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, &c)
	}
	return comments, rows.Err()
}
