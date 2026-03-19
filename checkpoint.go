package gig

import "fmt"

// AddCheckpoint creates a structured progress snapshot on a task.
func (s *Store) AddCheckpoint(taskID, author string, params CheckpointParams) (*Checkpoint, error) {
	if params.Done == "" {
		return nil, fmt.Errorf("checkpoint requires a 'done' summary")
	}

	if _, err := s.Get(taskID); err != nil {
		return nil, err
	}

	id := s.newID()
	now := timeNowUTC()

	cp := &Checkpoint{
		ID:        id,
		TaskID:    taskID,
		Author:    author,
		Done:      params.Done,
		Decisions: params.Decisions,
		Next:      params.Next,
		Blockers:  params.Blockers,
		Files:     params.Files,
		CreatedAt: now,
	}

	_, err := s.db.Exec(
		`INSERT INTO checkpoints (id, task_id, author, done, decisions, next, blockers, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, taskID, author, params.Done, params.Decisions, params.Next, params.Blockers, now.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("insert checkpoint: %w", err)
	}

	for _, f := range params.Files {
		_, err := s.db.Exec(
			"INSERT INTO checkpoint_files (checkpoint_id, file_path) VALUES (?, ?)",
			id, f,
		)
		if err != nil {
			return nil, fmt.Errorf("link file %s: %w", f, err)
		}
	}

	s.recordEvent(taskID, EventCommented, author, "", "", "checkpoint: "+params.Done)
	return cp, nil
}

// ListCheckpoints returns all checkpoints for a task, oldest first.
func (s *Store) ListCheckpoints(taskID string) ([]*Checkpoint, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, author, done, decisions, next, blockers, created_at
		 FROM checkpoints WHERE task_id = ? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()

	var cps []*Checkpoint
	for rows.Next() {
		var cp Checkpoint
		var createdAt string
		if err := rows.Scan(&cp.ID, &cp.TaskID, &cp.Author, &cp.Done, &cp.Decisions, &cp.Next, &cp.Blockers, &createdAt); err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}
		if t := strToTime(createdAt); t != nil {
			cp.CreatedAt = *t
		}

		files, err := s.checkpointFiles(cp.ID)
		if err != nil {
			return nil, err
		}
		cp.Files = files

		cps = append(cps, &cp)
	}
	return cps, rows.Err()
}

// LatestCheckpoint returns the most recent checkpoint for a task, or nil if none exist.
func (s *Store) LatestCheckpoint(taskID string) (*Checkpoint, error) {
	var cp Checkpoint
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, task_id, author, done, decisions, next, blockers, created_at
		 FROM checkpoints WHERE task_id = ? ORDER BY rowid DESC LIMIT 1`,
		taskID,
	).Scan(&cp.ID, &cp.TaskID, &cp.Author, &cp.Done, &cp.Decisions, &cp.Next, &cp.Blockers, &createdAt)
	if err != nil {
		return nil, nil // no checkpoints
	}
	if t := strToTime(createdAt); t != nil {
		cp.CreatedAt = *t
	}

	files, err := s.checkpointFiles(cp.ID)
	if err != nil {
		return nil, err
	}
	cp.Files = files

	return &cp, nil
}

// checkpointFiles returns file paths linked to a checkpoint.
func (s *Store) checkpointFiles(checkpointID string) ([]string, error) {
	rows, err := s.db.Query(
		"SELECT file_path FROM checkpoint_files WHERE checkpoint_id = ? ORDER BY rowid ASC",
		checkpointID,
	)
	if err != nil {
		return nil, fmt.Errorf("list checkpoint files: %w", err)
	}
	defer rows.Close()

	var files []string
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("scan file: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}
