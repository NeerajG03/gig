package gig

import (
	"testing"
)

func TestDefineAndListAttrDefs(t *testing.T) {
	store, _ := tempDB(t)

	if err := store.DefineAttr("worktree", AttrString, "Git worktree path"); err != nil {
		t.Fatalf("define: %v", err)
	}
	if err := store.DefineAttr("tested", AttrBoolean, "Tests passed"); err != nil {
		t.Fatalf("define: %v", err)
	}
	if err := store.DefineAttr("config", AttrObject, "Agent config JSON"); err != nil {
		t.Fatalf("define: %v", err)
	}

	defs, err := store.ListAttrDefs()
	if err != nil {
		t.Fatalf("list defs: %v", err)
	}
	if len(defs) != 3 {
		t.Fatalf("expected 3 defs, got %d", len(defs))
	}
	// Sorted by key: config, tested, worktree
	if defs[0].Key != "config" || defs[0].Type != AttrObject {
		t.Errorf("defs[0] = %q/%s, want config/object", defs[0].Key, defs[0].Type)
	}
}

func TestDefineAttrInvalidType(t *testing.T) {
	store, _ := tempDB(t)
	err := store.DefineAttr("foo", AttrType("invalid"), "")
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestDefineAttrUpsert(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("phase", AttrString, "old desc")
	store.DefineAttr("phase", AttrString, "new desc")

	def, err := store.GetAttrDef("phase")
	if err != nil {
		t.Fatalf("get def: %v", err)
	}
	if def.Description != "new desc" {
		t.Errorf("description = %q, want 'new desc'", def.Description)
	}
}

func TestSetAndGetStringAttr(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("worktree", AttrString, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	if err := store.SetAttr(task.ID, "worktree", "/tmp/wt-1"); err != nil {
		t.Fatalf("set: %v", err)
	}

	attr, err := store.GetAttr(task.ID, "worktree")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if attr.Value != "/tmp/wt-1" {
		t.Errorf("value = %q, want '/tmp/wt-1'", attr.Value)
	}
	if attr.Type != AttrString {
		t.Errorf("type = %q, want 'string'", attr.Type)
	}
	if attr.StringValue() != "/tmp/wt-1" {
		t.Errorf("StringValue() = %q", attr.StringValue())
	}
}

func TestSetAndGetBoolAttr(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("tested", AttrBoolean, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	if err := store.SetAttr(task.ID, "tested", "true"); err != nil {
		t.Fatalf("set: %v", err)
	}

	attr, err := store.GetAttr(task.ID, "tested")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !attr.BoolValue() {
		t.Error("BoolValue() should be true")
	}

	// Set to false
	store.SetAttr(task.ID, "tested", "false")
	attr, _ = store.GetAttr(task.ID, "tested")
	if attr.BoolValue() {
		t.Error("BoolValue() should be false after update")
	}
}

func TestSetBoolAttrRejectsInvalid(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("tested", AttrBoolean, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	err := store.SetAttr(task.ID, "tested", "yes")
	if err == nil {
		t.Error("expected error for invalid boolean value")
	}
}

func TestSetAndGetObjectAttr(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("config", AttrObject, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	jsonVal := `{"model":"opus","temp":0.7}`
	if err := store.SetAttr(task.ID, "config", jsonVal); err != nil {
		t.Fatalf("set: %v", err)
	}

	attr, err := store.GetAttr(task.ID, "config")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if attr.Value != jsonVal {
		t.Errorf("value = %q, want %q", attr.Value, jsonVal)
	}

	obj, err := attr.ObjectValue()
	if err != nil {
		t.Fatalf("ObjectValue: %v", err)
	}
	if obj["model"] != "opus" {
		t.Errorf("obj[model] = %v, want 'opus'", obj["model"])
	}
}

func TestSetObjectAttrRejectsInvalidJSON(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("config", AttrObject, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	err := store.SetAttr(task.ID, "config", "not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSetAttrRejectsUndefinedKey(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test"})

	err := store.SetAttr(task.ID, "undefined_key", "value")
	if err == nil {
		t.Error("expected error for undefined attribute key")
	}
}

func TestSetAttrUpsert(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("branch", AttrString, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	store.SetAttr(task.ID, "branch", "main")
	store.SetAttr(task.ID, "branch", "feature/x")

	attr, _ := store.GetAttr(task.ID, "branch")
	if attr.Value != "feature/x" {
		t.Errorf("value = %q, want 'feature/x'", attr.Value)
	}
}

func TestListAttrs(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("worktree", AttrString, "")
	store.DefineAttr("tested", AttrBoolean, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	store.SetAttr(task.ID, "worktree", "/tmp/wt")
	store.SetAttr(task.ID, "tested", "true")

	attrs, err := store.Attrs(task.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(attrs) != 2 {
		t.Fatalf("expected 2 attrs, got %d", len(attrs))
	}
}

func TestDeleteAttr(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("phase", AttrString, "")
	task, _ := store.Create(CreateParams{Title: "Test"})

	store.SetAttr(task.ID, "phase", "research")

	if err := store.DeleteAttr(task.ID, "phase"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := store.GetAttr(task.ID, "phase")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestDeleteAttrNotSet(t *testing.T) {
	store, _ := tempDB(t)
	task, _ := store.Create(CreateParams{Title: "Test"})

	err := store.DeleteAttr(task.ID, "nonexistent")
	if err == nil {
		t.Error("expected error for deleting non-existent attr")
	}
}

func TestUndefineAttrCascades(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("phase", AttrString, "")
	t1, _ := store.Create(CreateParams{Title: "T1"})
	t2, _ := store.Create(CreateParams{Title: "T2"})

	store.SetAttr(t1.ID, "phase", "research")
	store.SetAttr(t2.ID, "phase", "code")

	if err := store.UndefineAttr("phase"); err != nil {
		t.Fatalf("undefine: %v", err)
	}

	// All values should be gone.
	_, err := store.GetAttr(t1.ID, "phase")
	if err == nil {
		t.Error("expected error — value should be deleted")
	}
	_, err = store.GetAttr(t2.ID, "phase")
	if err == nil {
		t.Error("expected error — value should be deleted")
	}
}

func TestListFilterByAttr(t *testing.T) {
	store, _ := tempDB(t)
	store.DefineAttr("phase", AttrString, "")
	store.DefineAttr("tested", AttrBoolean, "")

	t1, _ := store.Create(CreateParams{Title: "Research task"})
	t2, _ := store.Create(CreateParams{Title: "Code task"})
	t3, _ := store.Create(CreateParams{Title: "Done task"})

	store.SetAttr(t1.ID, "phase", "research")
	store.SetAttr(t2.ID, "phase", "code")
	store.SetAttr(t3.ID, "phase", "code")
	store.SetAttr(t3.ID, "tested", "true")

	// Filter by phase=code
	results, err := store.List(ListParams{
		AttrFilter: map[string]string{"phase": "code"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 tasks with phase=code, got %d", len(results))
	}

	// Filter by phase=code AND tested=true
	results, err = store.List(ListParams{
		AttrFilter: map[string]string{"phase": "code", "tested": "true"},
	})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 task with phase=code AND tested=true, got %d", len(results))
	}
	if len(results) > 0 && results[0].ID != t3.ID {
		t.Errorf("expected task %s, got %s", t3.ID, results[0].ID)
	}
}
