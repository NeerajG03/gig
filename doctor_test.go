package gig

import (
	"testing"
)

func TestDoctorHealthyDB(t *testing.T) {
	store, _ := tempDB(t)

	store.Create(CreateParams{Title: "Task 1"})
	store.Create(CreateParams{Title: "Task 2"})

	report, err := store.Doctor()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}

	if report.HasIssues() {
		for _, d := range report.Diagnostics {
			if d.Level != DiagOK {
				t.Errorf("unexpected issue: [%s] %s: %s", d.Level, d.Check, d.Message)
			}
		}
	}

	// Should have diagnostics for each check.
	if len(report.Diagnostics) < 5 {
		t.Errorf("expected at least 5 diagnostics, got %d", len(report.Diagnostics))
	}
}

func TestDoctorNoConfig(t *testing.T) {
	store, _ := tempDB(t)

	report, _ := store.Doctor()
	found := false
	for _, d := range report.Diagnostics {
		if d.Check == "config" && d.Level == DiagOK {
			found = true
		}
	}
	if !found {
		t.Error("expected config check to pass with no config")
	}
}

func TestDoctorWithConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Prefix:      "test",
		HashLen:     4,
		DefaultView: "list",
	}
	store, err := Open(dir+"/test.db", WithPrefix("test"), WithConfig(cfg))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	report, _ := store.Doctor()
	for _, d := range report.Diagnostics {
		if d.Check == "config" && d.Level != DiagOK {
			t.Errorf("config issue with valid config: %s", d.Message)
		}
	}
}

func TestDoctorInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Prefix:      "",
		HashLen:     99,
		DefaultView: "invalid",
		Hooks: HookConfig{
			OnCreate: []HookDef{{Command: ""}},
		},
	}
	store, err := Open(dir+"/test.db", WithConfig(cfg))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	report, _ := store.Doctor()
	warnings := 0
	for _, d := range report.Diagnostics {
		if d.Level == DiagWarn {
			warnings++
		}
	}
	if warnings < 3 {
		t.Errorf("expected at least 3 warnings for bad config, got %d", warnings)
	}
}

func TestDoctorIntegrity(t *testing.T) {
	store, _ := tempDB(t)

	report, _ := store.Doctor()
	found := false
	for _, d := range report.Diagnostics {
		if d.Check == "integrity" && d.Level == DiagOK {
			found = true
		}
	}
	if !found {
		t.Error("expected integrity check to pass")
	}
}

func TestDoctorReportHasIssues(t *testing.T) {
	r := &DoctorReport{
		Diagnostics: []Diagnostic{
			{Level: DiagOK, Check: "a", Message: "ok"},
		},
	}
	if r.HasIssues() {
		t.Error("report with only OK should not have issues")
	}

	r.Diagnostics = append(r.Diagnostics, Diagnostic{Level: DiagWarn, Check: "b", Message: "warn"})
	if !r.HasIssues() {
		t.Error("report with warning should have issues")
	}
}
