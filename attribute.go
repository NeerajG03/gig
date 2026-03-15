package gig

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// DefineAttr registers a new attribute key with its type.
// This must be called before SetAttr can use that key.
func (s *Store) DefineAttr(key string, attrType AttrType, description string) error {
	if key == "" {
		return fmt.Errorf("attribute key is required")
	}
	if !attrType.IsValid() {
		return fmt.Errorf("invalid attribute type: %s", attrType)
	}

	now := timeNowUTC()
	_, err := s.db.Exec(
		`INSERT INTO attribute_definitions (key, attr_type, description, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET attr_type = ?, description = ?`,
		key, string(attrType), description, now.Format(timeFormat),
		string(attrType), description,
	)
	if err != nil {
		return fmt.Errorf("define attribute: %w", err)
	}
	return nil
}

// UndefineAttr removes an attribute definition and all its values across all tasks.
func (s *Store) UndefineAttr(key string) error {
	// Delete values first (FK would block otherwise if cascade isn't enough).
	if _, err := s.db.Exec("DELETE FROM custom_attributes WHERE key = ?", key); err != nil {
		return fmt.Errorf("delete attribute values: %w", err)
	}
	result, err := s.db.Exec("DELETE FROM attribute_definitions WHERE key = ?", key)
	if err != nil {
		return fmt.Errorf("delete attribute definition: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("attribute %q not defined", key)
	}
	return nil
}

// GetAttrDef retrieves a single attribute definition.
func (s *Store) GetAttrDef(key string) (*AttrDefinition, error) {
	var def AttrDefinition
	var createdAt string
	err := s.db.QueryRow(
		"SELECT key, attr_type, description, created_at FROM attribute_definitions WHERE key = ?", key,
	).Scan(&def.Key, &def.Type, &def.Description, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("attribute %q not defined", key)
		}
		return nil, fmt.Errorf("get attribute definition: %w", err)
	}
	if t := strToTime(createdAt); t != nil {
		def.CreatedAt = *t
	}
	return &def, nil
}

// ListAttrDefs returns all defined attribute types.
func (s *Store) ListAttrDefs() ([]*AttrDefinition, error) {
	rows, err := s.db.Query(
		"SELECT key, attr_type, description, created_at FROM attribute_definitions ORDER BY key",
	)
	if err != nil {
		return nil, fmt.Errorf("list attribute definitions: %w", err)
	}
	defer rows.Close()

	var defs []*AttrDefinition
	for rows.Next() {
		var def AttrDefinition
		var createdAt string
		if err := rows.Scan(&def.Key, &def.Type, &def.Description, &createdAt); err != nil {
			return nil, fmt.Errorf("scan attribute definition: %w", err)
		}
		if t := strToTime(createdAt); t != nil {
			def.CreatedAt = *t
		}
		defs = append(defs, &def)
	}
	return defs, rows.Err()
}

// SetAttr sets a custom attribute value on a task.
// The attribute key must be defined first via DefineAttr.
// The value is validated against the definition's type.
func (s *Store) SetAttr(taskID, key, value string) error {
	// Verify task exists.
	if _, err := s.Get(taskID); err != nil {
		return err
	}

	// Look up definition to validate type.
	def, err := s.GetAttrDef(key)
	if err != nil {
		return err
	}

	// Validate value against type.
	if err := validateAttrValue(def.Type, value); err != nil {
		return fmt.Errorf("attribute %q: %w", key, err)
	}

	now := timeNowUTC()
	nowStr := now.Format(timeFormat)

	// Check if exists for event recording.
	var oldValue string
	var isUpdate bool
	err = s.db.QueryRow(
		"SELECT value FROM custom_attributes WHERE task_id = ? AND key = ?", taskID, key,
	).Scan(&oldValue)
	if err == nil {
		isUpdate = true
	}

	_, err = s.db.Exec(
		`INSERT INTO custom_attributes (task_id, key, value, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(task_id, key) DO UPDATE SET value = ?, updated_at = ?`,
		taskID, key, value, nowStr, nowStr,
		value, nowStr,
	)
	if err != nil {
		return fmt.Errorf("set attribute: %w", err)
	}

	if isUpdate {
		s.recordEvent(taskID, EventUpdated, "", "attr:"+key, oldValue, value)
	} else {
		s.recordEvent(taskID, EventUpdated, "", "attr:"+key, "", value)
	}
	return nil
}

// GetAttr retrieves a single custom attribute for a task.
func (s *Store) GetAttr(taskID, key string) (*Attribute, error) {
	var attr Attribute
	var createdAt, updatedAt string
	err := s.db.QueryRow(
		`SELECT ca.task_id, ca.key, ca.value, ad.attr_type, ca.created_at, ca.updated_at
		 FROM custom_attributes ca
		 JOIN attribute_definitions ad ON ad.key = ca.key
		 WHERE ca.task_id = ? AND ca.key = ?`, taskID, key,
	).Scan(&attr.TaskID, &attr.Key, &attr.Value, &attr.Type, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("attribute %q not set on task %s", key, taskID)
		}
		return nil, fmt.Errorf("get attribute: %w", err)
	}
	if t := strToTime(createdAt); t != nil {
		attr.CreatedAt = *t
	}
	if t := strToTime(updatedAt); t != nil {
		attr.UpdatedAt = *t
	}
	return &attr, nil
}

// Attrs returns all custom attributes for a task.
func (s *Store) Attrs(taskID string) ([]*Attribute, error) {
	rows, err := s.db.Query(
		`SELECT ca.task_id, ca.key, ca.value, ad.attr_type, ca.created_at, ca.updated_at
		 FROM custom_attributes ca
		 JOIN attribute_definitions ad ON ad.key = ca.key
		 WHERE ca.task_id = ?
		 ORDER BY ca.key`, taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list attributes: %w", err)
	}
	defer rows.Close()

	var attrs []*Attribute
	for rows.Next() {
		var attr Attribute
		var createdAt, updatedAt string
		if err := rows.Scan(&attr.TaskID, &attr.Key, &attr.Value, &attr.Type, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan attribute: %w", err)
		}
		if t := strToTime(createdAt); t != nil {
			attr.CreatedAt = *t
		}
		if t := strToTime(updatedAt); t != nil {
			attr.UpdatedAt = *t
		}
		attrs = append(attrs, &attr)
	}
	return attrs, rows.Err()
}

// DeleteAttr removes a custom attribute from a task.
func (s *Store) DeleteAttr(taskID, key string) error {
	// Get old value for event.
	var oldValue string
	err := s.db.QueryRow(
		"SELECT value FROM custom_attributes WHERE task_id = ? AND key = ?", taskID, key,
	).Scan(&oldValue)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("attribute %q not set on task %s", key, taskID)
		}
		return fmt.Errorf("get attribute for delete: %w", err)
	}

	if _, err := s.db.Exec(
		"DELETE FROM custom_attributes WHERE task_id = ? AND key = ?", taskID, key,
	); err != nil {
		return fmt.Errorf("delete attribute: %w", err)
	}

	s.recordEvent(taskID, EventUpdated, "", "attr:"+key, oldValue, "")
	return nil
}

// validateAttrValue checks that a value matches the expected attribute type.
func validateAttrValue(attrType AttrType, value string) error {
	switch attrType {
	case AttrBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("boolean attribute must be \"true\" or \"false\", got %q", value)
		}
	case AttrObject:
		if !json.Valid([]byte(value)) {
			return fmt.Errorf("object attribute must be valid JSON")
		}
	case AttrString:
		// Any value is valid.
	}
	return nil
}
