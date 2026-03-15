package gig

import (
	"fmt"
)

// DiagnosticLevel indicates the severity of a diagnostic finding.
type DiagnosticLevel string

const (
	DiagOK   DiagnosticLevel = "ok"
	DiagWarn DiagnosticLevel = "warn"
	DiagFail DiagnosticLevel = "fail"
)

// Diagnostic represents a single health check result.
type Diagnostic struct {
	Level   DiagnosticLevel `json:"level"`
	Check   string          `json:"check"`
	Message string          `json:"message"`
}

// DoctorReport contains the full results of a health check.
type DoctorReport struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// HasIssues returns true if the report contains any warnings or failures.
func (r *DoctorReport) HasIssues() bool {
	for _, d := range r.Diagnostics {
		if d.Level != DiagOK {
			return true
		}
	}
	return false
}

// Doctor runs health checks on the store and returns a report.
func (s *Store) Doctor() (*DoctorReport, error) {
	report := &DoctorReport{}

	s.checkIntegrity(report)
	s.checkOrphanedParents(report)
	s.checkOrphanedComments(report)
	s.checkOrphanedDeps(report)
	s.checkOrphanedAttrs(report)
	s.checkDependencyCycles(report)
	s.checkConfigValues(report)
	s.checkTaskSummary(report)

	return report, nil
}

func (r *DoctorReport) add(level DiagnosticLevel, check, message string) {
	r.Diagnostics = append(r.Diagnostics, Diagnostic{Level: level, Check: check, Message: message})
}

func (s *Store) checkIntegrity(r *DoctorReport) {
	var result string
	err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		r.add(DiagFail, "integrity", fmt.Sprintf("integrity check error: %v", err))
		return
	}
	if result != "ok" {
		r.add(DiagFail, "integrity", fmt.Sprintf("database corruption: %s", result))
		return
	}
	r.add(DiagOK, "integrity", "database integrity OK")
}

func (s *Store) checkOrphanedParents(r *DoctorReport) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM tasks t
		 WHERE t.parent_id IS NOT NULL AND t.parent_id != ''
		   AND NOT EXISTS (SELECT 1 FROM tasks p WHERE p.id = t.parent_id)`,
	).Scan(&count)
	if err != nil {
		r.add(DiagFail, "orphaned_parents", fmt.Sprintf("query error: %v", err))
		return
	}
	if count > 0 {
		r.add(DiagWarn, "orphaned_parents", fmt.Sprintf("%d task(s) reference non-existent parent", count))
		return
	}
	r.add(DiagOK, "orphaned_parents", "no orphaned parent references")
}

func (s *Store) checkOrphanedComments(r *DoctorReport) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM comments c
		 WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = c.task_id)`,
	).Scan(&count)
	if err != nil {
		r.add(DiagFail, "orphaned_comments", fmt.Sprintf("query error: %v", err))
		return
	}
	if count > 0 {
		r.add(DiagWarn, "orphaned_comments", fmt.Sprintf("%d comment(s) reference non-existent tasks", count))
		return
	}
	r.add(DiagOK, "orphaned_comments", "no orphaned comments")
}

func (s *Store) checkOrphanedDeps(r *DoctorReport) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM dependencies d
		 WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = d.from_id)
		    OR NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = d.to_id)`,
	).Scan(&count)
	if err != nil {
		r.add(DiagFail, "orphaned_deps", fmt.Sprintf("query error: %v", err))
		return
	}
	if count > 0 {
		r.add(DiagWarn, "orphaned_deps", fmt.Sprintf("%d dependency(s) reference non-existent tasks", count))
		return
	}
	r.add(DiagOK, "orphaned_deps", "no orphaned dependencies")
}

func (s *Store) checkOrphanedAttrs(r *DoctorReport) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM custom_attributes ca
		 WHERE NOT EXISTS (SELECT 1 FROM tasks t WHERE t.id = ca.task_id)`,
	).Scan(&count)
	if err != nil {
		r.add(DiagFail, "orphaned_attrs", fmt.Sprintf("query error: %v", err))
		return
	}
	if count > 0 {
		r.add(DiagWarn, "orphaned_attrs", fmt.Sprintf("%d attribute(s) reference non-existent tasks", count))
		return
	}
	r.add(DiagOK, "orphaned_attrs", "no orphaned attributes")
}

func (s *Store) checkDependencyCycles(r *DoctorReport) {
	cycles, err := s.DetectCycles()
	if err != nil {
		r.add(DiagFail, "cycles", fmt.Sprintf("cycle detection error: %v", err))
		return
	}
	if len(cycles) > 0 {
		r.add(DiagWarn, "cycles", fmt.Sprintf("%d dependency cycle(s) found", len(cycles)))
		return
	}
	r.add(DiagOK, "cycles", "no dependency cycles")
}

func (s *Store) checkConfigValues(r *DoctorReport) {
	if s.config == nil {
		r.add(DiagOK, "config", "no config loaded (using defaults)")
		return
	}

	issues := 0
	if s.config.HashLen < 3 || s.config.HashLen > 8 {
		r.add(DiagWarn, "config", fmt.Sprintf("hash_length %d is out of range (3-8)", s.config.HashLen))
		issues++
	}
	if s.config.DefaultView != "" && s.config.DefaultView != "list" && s.config.DefaultView != "tree" {
		r.add(DiagWarn, "config", fmt.Sprintf("default_view %q is invalid (must be 'list' or 'tree')", s.config.DefaultView))
		issues++
	}
	if s.config.Prefix == "" {
		r.add(DiagWarn, "config", "prefix is empty")
		issues++
	}

	// Check hooks have non-empty commands.
	hookLists := map[string][]HookDef{
		"on_status_change": s.config.Hooks.OnStatusChange,
		"on_create":        s.config.Hooks.OnCreate,
		"on_comment":       s.config.Hooks.OnComment,
		"on_close":         s.config.Hooks.OnClose,
		"on_assign":        s.config.Hooks.OnAssign,
	}
	for name, hooks := range hookLists {
		for i, h := range hooks {
			if h.Command == "" {
				r.add(DiagWarn, "config", fmt.Sprintf("hook %s[%d] has empty command", name, i))
				issues++
			}
		}
	}

	if issues == 0 {
		r.add(DiagOK, "config", "configuration valid")
	}
}

func (s *Store) checkTaskSummary(r *DoctorReport) {
	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&total)
	if err != nil {
		r.add(DiagFail, "tasks", fmt.Sprintf("query error: %v", err))
		return
	}
	r.add(DiagOK, "tasks", fmt.Sprintf("%d task(s) in database", total))
}
