// Package migrate handles schema versioning for the gig SQLite database.
package migrate

import (
	"database/sql"
	"fmt"
)

// migration is a single schema change identified by version number.
type migration struct {
	Version int
	SQL     string
}

var migrations = []migration{
	{
		Version: 1,
		SQL: `
CREATE TABLE IF NOT EXISTS tasks (
    id              TEXT PRIMARY KEY,
    parent_id       TEXT,
    title           TEXT NOT NULL,
    description     TEXT DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open',
    priority        INTEGER NOT NULL DEFAULT 2,
    assignee        TEXT DEFAULT '',
    task_type       TEXT NOT NULL DEFAULT 'task',
    labels          TEXT DEFAULT '[]',
    notes           TEXT DEFAULT '',
    estimate        INTEGER DEFAULT 0,
    due_at          TEXT DEFAULT '',
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL,
    closed_at       TEXT DEFAULT '',
    close_reason    TEXT DEFAULT '',
    created_by      TEXT DEFAULT '',
    metadata        TEXT DEFAULT '',
    FOREIGN KEY (parent_id) REFERENCES tasks(id)
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);

CREATE TABLE IF NOT EXISTS comments (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL,
    author      TEXT DEFAULT '',
    content     TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_comments_task ON comments(task_id);

CREATE TABLE IF NOT EXISTS dependencies (
    from_id     TEXT NOT NULL,
    to_id       TEXT NOT NULL,
    dep_type    TEXT NOT NULL DEFAULT 'blocks',
    created_at  TEXT NOT NULL,
    PRIMARY KEY (from_id, to_id),
    FOREIGN KEY (from_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (to_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     TEXT NOT NULL,
    event_type  TEXT NOT NULL,
    actor       TEXT DEFAULT '',
    field       TEXT DEFAULT '',
    old_value   TEXT DEFAULT '',
    new_value   TEXT DEFAULT '',
    timestamp   TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id)
);

CREATE INDEX IF NOT EXISTS idx_events_task ON events(task_id);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(event_type);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);
`,
	},
	{
		Version: 2,
		SQL: `
CREATE TABLE IF NOT EXISTS attribute_definitions (
    key         TEXT PRIMARY KEY,
    attr_type   TEXT NOT NULL DEFAULT 'string',
    description TEXT DEFAULT '',
    created_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS custom_attributes (
    task_id     TEXT NOT NULL,
    key         TEXT NOT NULL,
    value       TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL,
    PRIMARY KEY (task_id, key),
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
    FOREIGN KEY (key) REFERENCES attribute_definitions(key)
);

CREATE INDEX IF NOT EXISTS idx_attrs_task ON custom_attributes(task_id);
CREATE INDEX IF NOT EXISTS idx_attrs_key_value ON custom_attributes(key, value);
`,
	},
	{
		Version: 3,
		SQL: `
CREATE TABLE IF NOT EXISTS checkpoints (
    id          TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL,
    author      TEXT DEFAULT '',
    done        TEXT NOT NULL,
    decisions   TEXT DEFAULT '',
    next        TEXT DEFAULT '',
    blockers    TEXT DEFAULT '',
    created_at  TEXT NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_checkpoints_task ON checkpoints(task_id);

CREATE TABLE IF NOT EXISTS checkpoint_files (
    checkpoint_id  TEXT NOT NULL,
    file_path      TEXT NOT NULL,
    FOREIGN KEY (checkpoint_id) REFERENCES checkpoints(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_checkpoint_files ON checkpoint_files(checkpoint_id);
`,
	},
}

// Run applies all pending migrations to the database.
func Run(db *sql.DB) error {
	// Create the migrations tracking table.
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		var exists int
		err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", m.Version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %d: %w", m.Version, err)
		}
		if exists > 0 {
			continue
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.Version, err)
		}

		if _, err := tx.Exec("INSERT INTO schema_migrations (version, applied_at) VALUES (?, datetime('now'))", m.Version); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.Version, err)
		}
	}

	return nil
}

// CurrentVersion returns the latest applied migration version.
func CurrentVersion(db *sql.DB) (int, error) {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}
