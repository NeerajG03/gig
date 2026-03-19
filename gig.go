// Package gig provides an embeddable task management system with
// SQLite storage, dependency tracking, and an event-driven hook system.
package gig

import (
	"encoding/json"
	"fmt"
	"time"
)

// Status represents the lifecycle state of a task.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusDeferred   Status = "deferred"
	StatusClosed     Status = "closed"
	StatusCancelled  Status = "cancelled"
)

// ValidStatuses is the set of all valid status values.
var ValidStatuses = []Status{StatusOpen, StatusInProgress, StatusBlocked, StatusDeferred, StatusClosed, StatusCancelled}

// IsTerminal returns true if the status represents a final state (closed or cancelled).
func (s Status) IsTerminal() bool {
	return s == StatusClosed || s == StatusCancelled
}

// IsValid returns true if s is a recognized status.
func (s Status) IsValid() bool {
	for _, v := range ValidStatuses {
		if s == v {
			return true
		}
	}
	return false
}

// Priority represents task urgency (0 = critical, 4 = backlog).
type Priority int

const (
	P0 Priority = 0 // Critical
	P1 Priority = 1 // High
	P2 Priority = 2 // Medium
	P3 Priority = 3 // Low
	P4 Priority = 4 // Backlog
)

// IsValid returns true if p is in the 0-4 range.
func (p Priority) IsValid() bool {
	return p >= P0 && p <= P4
}

func (p Priority) String() string {
	switch p {
	case P0:
		return "P0 (critical)"
	case P1:
		return "P1 (high)"
	case P2:
		return "P2 (medium)"
	case P3:
		return "P3 (low)"
	case P4:
		return "P4 (backlog)"
	default:
		return "unknown"
	}
}

// TaskType categorizes the nature of a task.
type TaskType string

const (
	TypeTask    TaskType = "task"
	TypeBug     TaskType = "bug"
	TypeFeature TaskType = "feature"
	TypeEpic    TaskType = "epic"
	TypeChore   TaskType = "chore"
)

// ValidTaskTypes is the set of all valid task type values.
var ValidTaskTypes = []TaskType{TypeTask, TypeBug, TypeFeature, TypeEpic, TypeChore}

// IsValid returns true if t is a recognized task type.
func (t TaskType) IsValid() bool {
	for _, v := range ValidTaskTypes {
		if t == v {
			return true
		}
	}
	return false
}

// DepType describes the relationship between two tasks.
type DepType string

const (
	Blocks    DepType = "blocks"
	RelatesTo DepType = "relates_to"
	Duplicates DepType = "duplicates"
)

// EventType describes what happened to a task.
type EventType string

const (
	EventCreated           EventType = "created"
	EventUpdated           EventType = "updated"
	EventStatusChanged     EventType = "status_changed"
	EventCommented         EventType = "commented"
	EventAssigned          EventType = "assigned"
	EventClosed            EventType = "closed"
	EventDependencyAdded   EventType = "dependency_added"
	EventDependencyRemoved EventType = "dependency_removed"
	EventDeleted           EventType = "deleted"
)

// Task is the primary entity in gig.
type Task struct {
	ID          string    `json:"id"`
	ParentID    string    `json:"parent_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      Status    `json:"status"`
	Priority    Priority  `json:"priority"`
	Assignee    string    `json:"assignee,omitempty"`
	Type        TaskType  `json:"type"`
	Labels      []string  `json:"labels,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	Estimate    int       `json:"estimate,omitempty"` // minutes
	DueAt       *time.Time `json:"due_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	CloseReason string    `json:"close_reason,omitempty"`
	CreatedBy   string    `json:"created_by,omitempty"`
	Metadata    string    `json:"metadata,omitempty"` // arbitrary JSON

	// Populated by GetTree — not stored in DB directly.
	Children []*Task `json:"children,omitempty"`
}

// Comment is a note attached to a task.
type Comment struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Author    string    `json:"author,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Checkpoint is a structured progress snapshot attached to a task.
type Checkpoint struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Author    string    `json:"author,omitempty"`
	Done      string    `json:"done"`
	Decisions string    `json:"decisions,omitempty"`
	Next      string    `json:"next,omitempty"`
	Blockers  string    `json:"blockers,omitempty"`
	Files     []string  `json:"files,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CheckpointParams holds the inputs for creating a checkpoint.
type CheckpointParams struct {
	Done      string   // What was accomplished
	Decisions string   // Key decisions and reasoning
	Next      string   // What should happen next
	Blockers  string   // Current blockers, if any
	Files     []string // File paths touched or referenced
}

// Dependency represents a directional relationship between tasks.
type Dependency struct {
	FromID    string    `json:"from_id"`
	ToID      string    `json:"to_id"`
	Type      DepType   `json:"type"`
	CreatedAt time.Time `json:"created_at"`
}

// Event records a change that happened to a task.
type Event struct {
	ID        int64     `json:"id"`
	TaskID    string    `json:"task_id"`
	Type      EventType `json:"event_type"`
	Actor     string    `json:"actor,omitempty"`
	Field     string    `json:"field,omitempty"`
	OldValue  string    `json:"old_value,omitempty"`
	NewValue  string    `json:"new_value,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// AttrType describes the data type of a custom attribute.
type AttrType string

const (
	AttrString  AttrType = "string"
	AttrBoolean AttrType = "boolean"
	AttrObject  AttrType = "object"
)

// ValidAttrTypes is the set of valid attribute types.
var ValidAttrTypes = []AttrType{AttrString, AttrBoolean, AttrObject}

// IsValid returns true if t is a recognized attribute type.
func (t AttrType) IsValid() bool {
	for _, v := range ValidAttrTypes {
		if t == v {
			return true
		}
	}
	return false
}

// AttrDefinition describes an allowed custom attribute key and its type.
type AttrDefinition struct {
	Key         string    `json:"key"`
	Type        AttrType  `json:"type"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Attribute is a typed key-value pair attached to a task.
type Attribute struct {
	TaskID    string    `json:"task_id"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	Type      AttrType  `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BoolValue returns the attribute value as a bool. Returns false if not "true".
func (a *Attribute) BoolValue() bool {
	return a.Value == "true"
}

// StringValue returns the raw string value.
func (a *Attribute) StringValue() string {
	return a.Value
}

// ObjectValue parses the JSON value into a map.
func (a *Attribute) ObjectValue() (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(a.Value), &m); err != nil {
		return nil, fmt.Errorf("parse object attribute %q: %w", a.Key, err)
	}
	return m, nil
}

// CreateParams holds the inputs for creating a new task.
type CreateParams struct {
	Title       string
	Description string
	Type        TaskType
	Priority    Priority
	ParentID    string
	Assignee    string
	Labels      []string
	Notes       string
	Estimate    int
	DueAt       *time.Time
	CreatedBy   string
	Metadata    string
}

// UpdateParams holds optional fields for updating a task.
// Pointer fields: nil means "don't change", non-nil means "set to this value".
type UpdateParams struct {
	Title       *string
	Description *string
	Priority    *Priority
	Assignee    *string
	Labels      *[]string
	Notes       *string
	Estimate    *int
	DueAt       *time.Time
	Metadata    *string
}

// ListParams controls filtering and pagination for List queries.
type ListParams struct {
	Status   *Status
	Assignee string
	Priority *Priority
	ParentID *string // pointer so we can distinguish "not set" from "root tasks"
	Type     *TaskType
	Label      string
	AttrFilter map[string]string // filter by custom attributes: key→value
	ExcludeStatuses []Status // tasks with these statuses are excluded
	Limit           int
	Offset          int
}
