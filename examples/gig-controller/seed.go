package main

import (
	"log"

	"github.com/neerajg/gig"
)

// seedDemoData populates the store with sample tasks if the DB is empty.
func seedDemoData() {
	all, _ := store.List(gig.ListParams{Limit: 1})
	if len(all) > 0 {
		return // already has data
	}

	log.Println("Seeding demo data...")

	// ── Epic 1: Platform Launch ──────────────────────────────────────
	launch, _ := store.Create(gig.CreateParams{
		Title:       "Platform Launch",
		Description: "Ship v1.0 of the platform with core features, infrastructure, and documentation.",
		Type:        gig.TypeEpic,
		Priority:    gig.P0,
		Assignee:    "neeraj",
		Labels:      []string{"launch", "q1"},
		CreatedBy:   "seed",
	})

	// Subtasks: launch.1, launch.2, launch.3, launch.4
	auth, _ := store.Create(gig.CreateParams{
		Title:       "Implement auth system",
		Description: "OAuth2 + JWT token flow with refresh tokens.",
		Type:        gig.TypeFeature,
		Priority:    gig.P0,
		ParentID:    launch.ID,
		Assignee:    "neeraj",
		Labels:      []string{"backend", "security"},
		CreatedBy:   "seed",
	})
	store.UpdateStatus(auth.ID, gig.StatusInProgress, "seed")

	api, _ := store.Create(gig.CreateParams{
		Title:       "Build REST API",
		Description: "CRUD endpoints for all core resources.",
		Type:        gig.TypeFeature,
		Priority:    gig.P1,
		ParentID:    launch.ID,
		Assignee:    "jeff",
		Labels:      []string{"backend", "api"},
		CreatedBy:   "seed",
	})
	store.UpdateStatus(api.ID, gig.StatusInProgress, "seed")

	ui, _ := store.Create(gig.CreateParams{
		Title:       "Design dashboard UI",
		Description: "Main dashboard with analytics widgets and navigation.",
		Type:        gig.TypeTask,
		Priority:    gig.P1,
		ParentID:    launch.ID,
		Assignee:    "priya",
		Labels:      []string{"frontend", "design"},
		CreatedBy:   "seed",
	})

	deploy, _ := store.Create(gig.CreateParams{
		Title:       "Setup CI/CD pipeline",
		Description: "GitHub Actions → Docker → ECS with staging and prod environments.",
		Type:        gig.TypeChore,
		Priority:    gig.P1,
		ParentID:    launch.ID,
		Assignee:    "neeraj",
		Labels:      []string{"infra", "devops"},
		CreatedBy:   "seed",
	})
	store.UpdateStatus(deploy.ID, gig.StatusBlocked, "seed")

	// Nested subtasks under auth: auth.1, auth.2, auth.3
	store.Create(gig.CreateParams{
		Title:    "Setup OAuth provider config",
		Type:     gig.TypeTask,
		Priority: gig.P1,
		ParentID: auth.ID,
		Assignee: "neeraj",
		Labels:   []string{"backend"},
		CreatedBy: "seed",
	})

	jwtTask, _ := store.Create(gig.CreateParams{
		Title:    "Implement JWT middleware",
		Type:     gig.TypeTask,
		Priority: gig.P0,
		ParentID: auth.ID,
		Assignee: "neeraj",
		Labels:   []string{"backend", "security"},
		CreatedBy: "seed",
	})
	store.UpdateStatus(jwtTask.ID, gig.StatusInProgress, "seed")

	refreshTask, _ := store.Create(gig.CreateParams{
		Title:    "Add refresh token rotation",
		Type:     gig.TypeTask,
		Priority: gig.P1,
		ParentID: auth.ID,
		Assignee: "neeraj",
		CreatedBy: "seed",
	})

	// Nested subtasks under API: api.1, api.2
	usersEndpoint, _ := store.Create(gig.CreateParams{
		Title:    "Users CRUD endpoints",
		Type:     gig.TypeTask,
		Priority: gig.P1,
		ParentID: api.ID,
		Assignee: "jeff",
		Labels:   []string{"backend"},
		CreatedBy: "seed",
	})
	store.UpdateStatus(usersEndpoint.ID, gig.StatusClosed, "seed")
	store.CloseTask(usersEndpoint.ID, "completed", "seed")

	store.Create(gig.CreateParams{
		Title:    "Projects CRUD endpoints",
		Type:     gig.TypeTask,
		Priority: gig.P1,
		ParentID: api.ID,
		Assignee: "jeff",
		Labels:   []string{"backend"},
		CreatedBy: "seed",
	})

	// ── Epic 2: Observability ────────────────────────────────────────
	obs, _ := store.Create(gig.CreateParams{
		Title:       "Observability & Monitoring",
		Description: "Logging, metrics, and alerting infrastructure.",
		Type:        gig.TypeEpic,
		Priority:    gig.P1,
		Assignee:    "jeff",
		Labels:      []string{"infra", "observability"},
		CreatedBy:   "seed",
	})

	logging, _ := store.Create(gig.CreateParams{
		Title:    "Structured logging setup",
		Type:     gig.TypeTask,
		Priority: gig.P1,
		ParentID: obs.ID,
		Assignee: "jeff",
		Labels:   []string{"backend"},
		CreatedBy: "seed",
	})
	store.UpdateStatus(logging.ID, gig.StatusClosed, "seed")
	store.CloseTask(logging.ID, "shipped in v0.9", "seed")

	metrics, _ := store.Create(gig.CreateParams{
		Title:       "Prometheus metrics integration",
		Description: "Instrument HTTP handlers and DB queries with Prometheus counters/histograms.",
		Type:        gig.TypeFeature,
		Priority:    gig.P2,
		ParentID:    obs.ID,
		Assignee:    "jeff",
		Labels:      []string{"backend", "metrics"},
		CreatedBy:   "seed",
	})

	store.Create(gig.CreateParams{
		Title:    "Grafana dashboard templates",
		Type:     gig.TypeTask,
		Priority: gig.P3,
		ParentID: obs.ID,
		Labels:   []string{"infra", "dashboards"},
		CreatedBy: "seed",
	})
	store.UpdateStatus(obs.ID, gig.StatusInProgress, "seed")

	// ── Standalone tasks (no parent) ─────────────────────────────────
	bug, _ := store.Create(gig.CreateParams{
		Title:       "Fix memory leak in websocket handler",
		Description: "Connection pool not releasing idle connections after timeout.",
		Type:        gig.TypeBug,
		Priority:    gig.P0,
		Assignee:    "neeraj",
		Labels:      []string{"backend", "critical"},
		CreatedBy:   "seed",
	})
	store.UpdateStatus(bug.ID, gig.StatusInProgress, "seed")

	store.Create(gig.CreateParams{
		Title:       "Evaluate message queue options",
		Description: "Compare NATS, RabbitMQ, and Redis Streams for async job processing.",
		Type:        gig.TypeTask,
		Priority:    gig.P3,
		Labels:      []string{"research", "infra"},
		CreatedBy:   "seed",
	})
	store.UpdateStatus(ui.ID, gig.StatusDeferred, "seed")

	store.Create(gig.CreateParams{
		Title:       "Write API documentation",
		Description: "OpenAPI spec + usage examples for all public endpoints.",
		Type:        gig.TypeChore,
		Priority:    gig.P2,
		Assignee:    "priya",
		Labels:      []string{"docs"},
		CreatedBy:   "seed",
	})

	migrationBug, _ := store.Create(gig.CreateParams{
		Title:       "Database migration fails on empty schema",
		Description: "First-time setup crashes when schema_migrations table doesn't exist.",
		Type:        gig.TypeBug,
		Priority:    gig.P1,
		Assignee:    "jeff",
		Labels:      []string{"backend", "bug"},
		CreatedBy:   "seed",
	})
	store.CloseTask(migrationBug.ID, "fixed in commit abc123", "seed")

	// ── Dependencies ─────────────────────────────────────────────────
	// CI/CD blocked by auth completion
	store.AddDependency(deploy.ID, auth.ID, gig.Blocks)
	// Refresh tokens depend on JWT middleware
	store.AddDependency(refreshTask.ID, jwtTask.ID, gig.Blocks)
	// Metrics depends on logging
	store.AddDependency(metrics.ID, logging.ID, gig.Blocks)

	// ── Comments ─────────────────────────────────────────────────────
	store.AddComment(auth.ID, "neeraj", "Started with Google OAuth, will add GitHub later.")
	store.AddComment(auth.ID, "jeff", "Make sure to handle token expiry edge cases.")
	store.AddComment(bug.ID, "neeraj", "Reproduced locally — goroutine leak in the upgrade handler.")
	store.AddComment(deploy.ID, "priya", "Can we use the existing Terraform modules?")
	store.AddComment(launch.ID, "neeraj", "Targeting end of Q1 for v1.0 release.")

	log.Println("Demo data seeded successfully.")
}
