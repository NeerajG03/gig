package gig

import (
	"fmt"
	"strings"
)

// AddDependency creates a dependency: fromID depends on toID.
// It checks for cycles before inserting.
func (s *Store) AddDependency(fromID, toID string, depType DepType) error {
	if fromID == toID {
		return fmt.Errorf("a task cannot depend on itself")
	}

	// Verify both tasks exist.
	if _, err := s.Get(fromID); err != nil {
		return fmt.Errorf("from task: %w", err)
	}
	if _, err := s.Get(toID); err != nil {
		return fmt.Errorf("to task: %w", err)
	}

	// Check for existing dependency.
	var exists int
	if err := s.db.QueryRow(
		"SELECT COUNT(*) FROM dependencies WHERE from_id = ? AND to_id = ?",
		fromID, toID,
	).Scan(&exists); err != nil {
		return fmt.Errorf("check existing dep: %w", err)
	}
	if exists > 0 {
		return nil // already exists
	}

	// Cycle detection: would adding fromID->toID create a cycle?
	if s.wouldCreateCycle(fromID, toID) {
		return fmt.Errorf("adding dependency %s -> %s would create a cycle", fromID, toID)
	}

	now := timeNowUTC()
	_, err := s.db.Exec(
		"INSERT INTO dependencies (from_id, to_id, dep_type, created_at) VALUES (?, ?, ?, ?)",
		fromID, toID, string(depType), now.Format(timeFormat),
	)
	if err != nil {
		return fmt.Errorf("insert dependency: %w", err)
	}

	s.recordEvent(fromID, EventDependencyAdded, "", "dependency", "", toID)
	return nil
}

// RemoveDependency removes a dependency between two tasks.
func (s *Store) RemoveDependency(fromID, toID string) error {
	result, err := s.db.Exec(
		"DELETE FROM dependencies WHERE from_id = ? AND to_id = ?",
		fromID, toID,
	)
	if err != nil {
		return fmt.Errorf("delete dependency: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		s.recordEvent(fromID, EventDependencyRemoved, "", "dependency", toID, "")
	}
	return nil
}

// ListDependencies returns what a task depends on (its blockers).
func (s *Store) ListDependencies(id string) ([]*Dependency, error) {
	return s.queryDeps("SELECT from_id, to_id, dep_type, created_at FROM dependencies WHERE from_id = ?", id)
}

// ListDependents returns tasks that depend on the given task.
func (s *Store) ListDependents(id string) ([]*Dependency, error) {
	return s.queryDeps("SELECT from_id, to_id, dep_type, created_at FROM dependencies WHERE to_id = ?", id)
}

// DepTree returns an ASCII tree visualization of a task's dependency chain.
func (s *Store) DepTree(id string) (string, error) {
	task, err := s.Get(id)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	s.buildDepTree(&sb, task, "", true, make(map[string]bool))
	return sb.String(), nil
}

// DetectCycles finds all cycles in the dependency graph.
// Returns a slice of cycles, where each cycle is a slice of task IDs.
func (s *Store) DetectCycles() ([][]string, error) {
	// Build adjacency list.
	rows, err := s.db.Query("SELECT from_id, to_id FROM dependencies WHERE dep_type = 'blocks'")
	if err != nil {
		return nil, fmt.Errorf("query dependencies: %w", err)
	}
	defer rows.Close()

	graph := make(map[string][]string)
	nodes := make(map[string]bool)
	for rows.Next() {
		var from, to string
		if err := rows.Scan(&from, &to); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		graph[from] = append(graph[from], to)
		nodes[from] = true
		nodes[to] = true
	}

	var cycles [][]string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var dfs func(node string, path []string)
	dfs = func(node string, path []string) {
		visited[node] = true
		inStack[node] = true
		path = append(path, node)

		for _, neighbor := range graph[node] {
			if inStack[neighbor] {
				// Found cycle — extract it.
				cycleStart := -1
				for i, n := range path {
					if n == neighbor {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path[cycleStart:]))
					copy(cycle, path[cycleStart:])
					cycles = append(cycles, cycle)
				}
			} else if !visited[neighbor] {
				dfs(neighbor, path)
			}
		}

		inStack[node] = false
	}

	for node := range nodes {
		if !visited[node] {
			dfs(node, nil)
		}
	}

	return cycles, nil
}

// wouldCreateCycle checks if adding from->to would create a cycle.
// It does BFS from toID looking for fromID.
func (s *Store) wouldCreateCycle(fromID, toID string) bool {
	visited := make(map[string]bool)
	queue := []string{toID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == fromID {
			return true
		}

		if visited[current] {
			continue
		}
		visited[current] = true

		rows, err := s.db.Query(
			"SELECT to_id FROM dependencies WHERE from_id = ? AND dep_type = 'blocks'",
			current,
		)
		if err != nil {
			continue
		}
		for rows.Next() {
			var next string
			if err := rows.Scan(&next); err == nil {
				queue = append(queue, next)
			}
		}
		rows.Close()
	}

	return false
}

func (s *Store) buildDepTree(sb *strings.Builder, task *Task, prefix string, isLast bool, visited map[string]bool) {
	connector := "|- "
	if isLast {
		connector = "`- "
	}
	if prefix == "" {
		connector = ""
	}

	statusIcon := " "
	switch task.Status {
	case StatusClosed:
		statusIcon = "x"
	case StatusInProgress:
		statusIcon = ">"
	case StatusBlocked:
		statusIcon = "!"
	}

	sb.WriteString(fmt.Sprintf("%s%s[%s] %s (%s)\n", prefix, connector, statusIcon, task.Title, task.ID))

	if visited[task.ID] {
		return
	}
	visited[task.ID] = true

	deps, _ := s.ListDependencies(task.ID)
	for i, dep := range deps {
		depTask, err := s.Get(dep.ToID)
		if err != nil {
			continue
		}
		childPrefix := prefix
		if prefix != "" {
			if isLast {
				childPrefix += "   "
			} else {
				childPrefix += "|  "
			}
		}
		s.buildDepTree(sb, depTask, childPrefix, i == len(deps)-1, visited)
	}
}

func (s *Store) queryDeps(query, id string) ([]*Dependency, error) {
	rows, err := s.db.Query(query, id)
	if err != nil {
		return nil, fmt.Errorf("query dependencies: %w", err)
	}
	defer rows.Close()

	var deps []*Dependency
	for rows.Next() {
		var d Dependency
		var createdAt string
		if err := rows.Scan(&d.FromID, &d.ToID, &d.Type, &createdAt); err != nil {
			return nil, fmt.Errorf("scan dependency: %w", err)
		}
		if ct := strToTime(createdAt); ct != nil {
			d.CreatedAt = *ct
		}
		deps = append(deps, &d)
	}
	return deps, rows.Err()
}
