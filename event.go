package gig

import (
	"fmt"
	"time"
)

// Events returns all events for a specific task, oldest first.
//
// Ordered by (timestamp, id) so events sharing a timestamp — RFC3339 has only
// second precision — come back in insertion order rather than an unspecified one.
func (s *Store) Events(taskID string) ([]*Event, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, event_type, actor, field, old_value, new_value, timestamp
		 FROM events WHERE task_id = ? ORDER BY timestamp ASC, id ASC`, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// EventsSince returns all events after the given timestamp, oldest first.
//
// Because timestamps have only second precision, a consumer polling with a time
// cursor can miss or duplicate events sharing the boundary second. Prefer
// EventsAfterID for a reliable resume cursor. Ordered by (timestamp, id).
func (s *Store) EventsSince(since time.Time) ([]*Event, error) {
	rows, err := s.db.Query(
		`SELECT id, task_id, event_type, actor, field, old_value, new_value, timestamp
		 FROM events WHERE timestamp > ? ORDER BY timestamp ASC, id ASC`,
		since.Format(timeFormat),
	)
	if err != nil {
		return nil, fmt.Errorf("query events since: %w", err)
	}
	defer rows.Close()

	return s.scanEvents(rows)
}

// EventsAfterID returns up to limit events with id > afterID, ordered by id.
// The id is a monotonically increasing integer suitable as a resume cursor:
// persist the last id you processed and pass it back to continue exactly where
// you left off, with no missed or duplicated events (unlike a timestamp cursor).
// A limit <= 0 means no limit. Pass afterID = 0 to start from the beginning.
func (s *Store) EventsAfterID(afterID int64, limit int) ([]*Event, error) {
	query := `SELECT id, task_id, event_type, actor, field, old_value, new_value, timestamp
		 FROM events WHERE id > ? ORDER BY id ASC`
	args := []any{afterID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events after id: %w", err)
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
		if t := strToTime(ts); t != nil {
			e.Timestamp = *t
		}
		events = append(events, &e)
	}
	return events, rows.Err()
}
