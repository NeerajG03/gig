package gig

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/NeerajG03/gig/internal/migrate"
	_ "modernc.org/sqlite"
)

// defaultHashLength is the hash-portion length of generated IDs. 6 hex chars =
// 16.7M values (e.g. "gig-a3f8c1"), keeping the birthday-bound collision
// probability low into the tens of thousands of tasks. Existing shorter IDs
// stay valid — length only affects newly generated IDs.
const defaultHashLength = 6

// GenerateID produces a short, prefix-based ID like "gig-a3f8c1".
// The hash is derived from a UUID + current timestamp to ensure uniqueness.
func GenerateID(prefix string, hashLen int) string {
	if prefix == "" {
		prefix = "gig"
	}
	if hashLen < 3 || hashLen > 8 {
		hashLen = defaultHashLength
	}

	raw := fmt.Sprintf("%s-%d", uuid.New().String(), time.Now().UnixNano())
	sum := sha256.Sum256([]byte(raw))
	hash := hex.EncodeToString(sum[:])[:hashLen]

	return fmt.Sprintf("%s-%s", prefix, hash)
}

// Store is the main entry point for the gig SDK.
// It holds the database connection, configuration, and event listeners.
type Store struct {
	db        *sql.DB
	prefix    string
	hashLen   int
	config    *Config
	listeners map[EventType][]func(Event)
	mu        sync.RWMutex // protects listeners
}

// Option configures a Store during Open.
type Option func(*Store)

// WithPrefix sets the ID prefix (default: "gig").
func WithPrefix(p string) Option {
	return func(s *Store) {
		if p != "" {
			s.prefix = p
		}
	}
}

// WithHashLength sets the hash portion length of generated IDs (3-8, default: 6).
func WithHashLength(n int) Option {
	return func(s *Store) {
		if n >= 3 && n <= 8 {
			s.hashLen = n
		}
	}
}

// WithConfig attaches a parsed Config to the store.
func WithConfig(c *Config) Option {
	return func(s *Store) {
		s.config = c
	}
}

// Open creates or opens a gig database at the given path.
// It runs pending migrations and enables WAL mode and foreign keys.
//
// PRAGMAs are applied via the DSN's _pragma query params so they bind to
// *every* pooled connection (not just whichever one ran a one-off PRAGMA
// exec). SetMaxOpenConns(1) additionally serializes Go-side access, matching
// SQLite's single-writer reality and making multi-statement sequences
// race-free within a process. busy_timeout(5000) covers cross-process
// contention (multi-agent use).
//
// Note: the "file:" DSN treats '?' and '#' in dbPath as query/fragment
// delimiters. A path containing those characters will be misparsed; escaping
// them breaks '/' handling, so such paths are simply unsupported.
func Open(dbPath string, opts ...Option) (*Store, error) {
	// Ensure parent directory exists.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	// modernc.org/sqlite applies _pragma query params to every new connection.
	dsn := "file:" + dbPath + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Serialize Go-side access: SQLite is single-writer, and a single conn
	// makes COUNT+INSERT and other multi-statement sequences race-free in-process.
	db.SetMaxOpenConns(1)

	// Run migrations.
	if err := migrate.Run(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	s := &Store{
		db:        db,
		prefix:    "gig",
		hashLen:   defaultHashLength,
		listeners: make(map[EventType][]func(Event)),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying *sql.DB for advanced use cases.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Prefix returns the configured ID prefix.
func (s *Store) Prefix() string {
	return s.prefix
}

// newID generates a new unique ID using the store's prefix and hash length.
func (s *Store) newID() string {
	return GenerateID(s.prefix, s.hashLen)
}

// emit fires all registered listeners for the given event type.
//
// Listeners run synchronously on the writing goroutine — keep them fast; a slow
// callback blocks the mutation that triggered it. Emission is in-process only:
// other processes writing to the same database file do NOT trigger these
// listeners (use EventsAfterID as a cross-process sweep instead).
func (s *Store) emit(e Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, fn := range s.listeners[e.Type] {
		fn(e)
	}
}

// On registers a callback for the given event type.
//
// Callbacks run synchronously on the writing goroutine and in-process only —
// see emit. Keep them fast; offload slow work to your own goroutine/queue.
func (s *Store) On(eventType EventType, fn func(Event)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners[eventType] = append(s.listeners[eventType], fn)
}

// Off removes all callbacks for the given event type.
func (s *Store) Off(eventType EventType) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.listeners, eventType)
}

const insertEventSQL = `INSERT INTO events (task_id, event_type, actor, field, old_value, new_value, timestamp)
	 VALUES (?, ?, ?, ?, ?, ?, ?)`

// recordEvent inserts an event into the events table and emits it to listeners.
//
// The INSERT error is returned rather than discarded (finding #3: a silently
// dropped SQLITE_BUSY meant an audit record vanished with no trace). Callers on
// the non-transactional emit path may choose log-and-continue, but the failure
// is also surfaced here via slog so it is never invisible.
func (s *Store) recordEvent(taskID string, eventType EventType, actor, field, oldVal, newVal string) error {
	now := timeNowUTC()
	_, err := s.db.Exec(insertEventSQL,
		taskID, string(eventType), actor, field, oldVal, newVal, now.Format(timeFormat),
	)
	if err != nil {
		slog.Warn("gig: failed to record event",
			"task_id", taskID, "event_type", string(eventType), "err", err)
		return fmt.Errorf("record event: %w", err)
	}

	s.emit(Event{
		TaskID:    taskID,
		Type:      eventType,
		Actor:     actor,
		Field:     field,
		OldValue:  oldVal,
		NewValue:  newVal,
		Timestamp: now,
	})
	return nil
}

// recordEventTx inserts an event using the given transaction. Unlike recordEvent
// it does NOT emit — the caller must emit only after the transaction commits, so
// listeners never observe events for a mutation that later rolled back. It
// returns an Event describing what was written so the caller can emit it.
func (s *Store) recordEventTx(tx *sql.Tx, taskID string, eventType EventType, actor, field, oldVal, newVal string) (Event, error) {
	now := timeNowUTC()
	_, err := tx.Exec(insertEventSQL,
		taskID, string(eventType), actor, field, oldVal, newVal, now.Format(timeFormat),
	)
	e := Event{
		TaskID:    taskID,
		Type:      eventType,
		Actor:     actor,
		Field:     field,
		OldValue:  oldVal,
		NewValue:  newVal,
		Timestamp: now,
	}
	if err != nil {
		return e, fmt.Errorf("record event: %w", err)
	}
	return e, nil
}
