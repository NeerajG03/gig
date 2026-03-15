package gig

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/neerajg/gig/internal/migrate"
	_ "modernc.org/sqlite"
)

const defaultHashLength = 4

// GenerateID produces a short, prefix-based ID like "gig-a3f8".
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

// WithHashLength sets the hash portion length of generated IDs (3-8, default: 4).
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
func Open(dbPath string, opts ...Option) (*Store, error) {
	// Ensure parent directory exists.
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Run migrations.
	if err := migrate.Run(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	s := &Store{
		db:        db,
		prefix:    "gig",
		hashLen:   4,
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
func (s *Store) emit(e Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, fn := range s.listeners[e.Type] {
		fn(e)
	}
}

// On registers a callback for the given event type.
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

// recordEvent inserts an event into the events table and emits it to listeners.
func (s *Store) recordEvent(taskID string, eventType EventType, actor, field, oldVal, newVal string) {
	now := timeNowUTC()
	_, _ = s.db.Exec(
		`INSERT INTO events (task_id, event_type, actor, field, old_value, new_value, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, string(eventType), actor, field, oldVal, newVal, now.Format(timeFormat),
	)

	s.emit(Event{
		TaskID:    taskID,
		Type:      eventType,
		Actor:     actor,
		Field:     field,
		OldValue:  oldVal,
		NewValue:  newVal,
		Timestamp: now,
	})
}
