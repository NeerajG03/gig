package gig

import (
	"testing"
)

func TestAddAndListDependency(t *testing.T) {
	store, _ := tempDB(t)
	a, _ := store.Create(CreateParams{Title: "A"})
	b, _ := store.Create(CreateParams{Title: "B"})

	if err := store.AddDependency(b.ID, a.ID, Blocks); err != nil {
		t.Fatalf("add dep: %v", err)
	}

	deps, err := store.ListDependencies(b.ID)
	if err != nil {
		t.Fatalf("list deps: %v", err)
	}
	if len(deps) != 1 || deps[0].ToID != a.ID {
		t.Errorf("expected dep on %s, got %v", a.ID, deps)
	}

	dependents, err := store.ListDependents(a.ID)
	if err != nil {
		t.Fatalf("list dependents: %v", err)
	}
	if len(dependents) != 1 || dependents[0].FromID != b.ID {
		t.Errorf("expected dependent %s, got %v", b.ID, dependents)
	}
}

func TestAddDuplicateDependency(t *testing.T) {
	store, _ := tempDB(t)
	a, _ := store.Create(CreateParams{Title: "A"})
	b, _ := store.Create(CreateParams{Title: "B"})

	store.AddDependency(b.ID, a.ID, Blocks)
	// Adding again should be a no-op.
	if err := store.AddDependency(b.ID, a.ID, Blocks); err != nil {
		t.Fatalf("duplicate dep should not error: %v", err)
	}
}

func TestSelfDependency(t *testing.T) {
	store, _ := tempDB(t)
	a, _ := store.Create(CreateParams{Title: "A"})

	err := store.AddDependency(a.ID, a.ID, Blocks)
	if err == nil {
		t.Error("expected error for self-dependency")
	}
}

func TestCycleDetection(t *testing.T) {
	store, _ := tempDB(t)
	a, _ := store.Create(CreateParams{Title: "A"})
	b, _ := store.Create(CreateParams{Title: "B"})
	c, _ := store.Create(CreateParams{Title: "C"})

	store.AddDependency(b.ID, a.ID, Blocks) // B depends on A
	store.AddDependency(c.ID, b.ID, Blocks) // C depends on B

	// A depends on C would create cycle: A -> C -> B -> A
	err := store.AddDependency(a.ID, c.ID, Blocks)
	if err == nil {
		t.Error("expected cycle detection error")
	}
}

func TestRemoveDependency(t *testing.T) {
	store, _ := tempDB(t)
	a, _ := store.Create(CreateParams{Title: "A"})
	b, _ := store.Create(CreateParams{Title: "B"})

	store.AddDependency(b.ID, a.ID, Blocks)
	if err := store.RemoveDependency(b.ID, a.ID); err != nil {
		t.Fatalf("remove dep: %v", err)
	}

	deps, _ := store.ListDependencies(b.ID)
	if len(deps) != 0 {
		t.Error("expected no dependencies after removal")
	}
}

func TestDepTree(t *testing.T) {
	store, _ := tempDB(t)
	a, _ := store.Create(CreateParams{Title: "Foundation"})
	b, _ := store.Create(CreateParams{Title: "Walls"})
	c, _ := store.Create(CreateParams{Title: "Roof"})

	store.AddDependency(b.ID, a.ID, Blocks)
	store.AddDependency(c.ID, b.ID, Blocks)

	tree, err := store.DepTree(c.ID)
	if err != nil {
		t.Fatalf("dep tree: %v", err)
	}
	if tree == "" {
		t.Error("expected non-empty tree output")
	}
	t.Log("Dep tree:\n" + tree)
}

func TestDetectCyclesEmpty(t *testing.T) {
	store, _ := tempDB(t)
	store.Create(CreateParams{Title: "A"})
	store.Create(CreateParams{Title: "B"})

	cycles, err := store.DetectCycles()
	if err != nil {
		t.Fatalf("detect cycles: %v", err)
	}
	if len(cycles) != 0 {
		t.Errorf("expected no cycles, got %v", cycles)
	}
}
