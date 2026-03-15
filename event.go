package gig

import (
	"fmt"
	"time"
)

// Events returns all events for a specific task, oldest first.
func (s *Store) Events(taskID string) ([]*Event, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, event_type, actor, field, old_value, new_value, timestamp
		 FROM events WHERE task_id = ? ORDER BY timestamp ASC`, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// EventsSince returns all events after the given timestamp.
func (s *Store) EventsSince(since time.Time) ([]*Event, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, event_type, actor, field, old_value, new_value, timestamp
		 FROM events WHERE timestamp > ? ORDER BY timestamp ASC`,
		since.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("query events since: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

func (s *Store) scanEvents(rows interface{ Next() bool; Scan(...any) error; Err() error }) ([]*Event, error) {
	var events []*Event
	for rows.Next() {
		var e Event
		var ts string
		if err := rows.Scan(&e.ID, &e.TaskID, &e.Type, &e.Actor, &e.Field, &e.OldValue, &e.NewValue, &ts); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		if t, err := time.Parse(timeFormat, ts); err == nil {
			e.Timestamp = t
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}
