// gig-controller is a web-based kanban board that demonstrates the gig SDK.
// Run: GIG_HOME=/tmp/gig-demo go run .
// Open: http://localhost:9741
package main

import (
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"

	"github.com/neerajg/gig"
)

//go:embed templates/*
var templateFS embed.FS

var (
	store *gig.Store
	tmpl  *template.Template
)

func main() {
	// Initialize gig store.
	cfg, err := gig.LoadConfig("")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Auto-init if no DB exists.
	if _, err := os.Stat(cfg.DBPath); os.IsNotExist(err) {
		if err := gig.SaveConfig("", cfg); err != nil {
			log.Fatalf("save config: %v", err)
		}
	}

	store, err = gig.Open(cfg.DBPath, gig.WithPrefix(cfg.Prefix), gig.WithHashLength(cfg.HashLen))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// Seed demo data if DB is empty.
	seedDemoData()

	// Parse templates with helper functions.
	funcMap := template.FuncMap{
		"statusIcon": func(s gig.Status) string {
			switch s {
			case gig.StatusOpen:
				return "○"
			case gig.StatusInProgress:
				return "◉"
			case gig.StatusBlocked:
				return "⊘"
			case gig.StatusDeferred:
				return "◌"
			case gig.StatusClosed:
				return "✓"
			default:
				return "?"
			}
		},
		"priorityClass": func(p gig.Priority) string {
			switch p {
			case gig.P0:
				return "priority-critical"
			case gig.P1:
				return "priority-high"
			case gig.P2:
				return "priority-medium"
			case gig.P3:
				return "priority-low"
			case gig.P4:
				return "priority-backlog"
			default:
				return ""
			}
		},
		"priorityLabel": func(p gig.Priority) string {
			switch p {
			case gig.P0:
				return "P0"
			case gig.P1:
				return "P1"
			case gig.P2:
				return "P2"
			case gig.P3:
				return "P3"
			case gig.P4:
				return "P4"
			default:
				return "?"
			}
		},
		"statusLabel": func(s gig.Status) string {
			switch s {
			case gig.StatusOpen:
				return "Open"
			case gig.StatusInProgress:
				return "In Progress"
			case gig.StatusBlocked:
				return "Blocked"
			case gig.StatusDeferred:
				return "Deferred"
			case gig.StatusClosed:
				return "Closed"
			default:
				return string(s)
			}
		},
		"columnColor": func(s gig.Status) string {
			switch s {
			case gig.StatusOpen:
				return "#6b7280"
			case gig.StatusInProgress:
				return "#3b82f6"
			case gig.StatusBlocked:
				return "#ef4444"
			case gig.StatusDeferred:
				return "#f59e0b"
			case gig.StatusClosed:
				return "#22c55e"
			default:
				return "#6b7280"
			}
		},
	}

	tmpl, err = template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html", "templates/partials/*.html")
	if err != nil {
		log.Fatalf("parse templates: %v", err)
	}

	// Routes.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleBoard)
	mux.HandleFunc("GET /task/{id}", handleDetail)
	mux.HandleFunc("GET /new", handleCreateForm)
	mux.HandleFunc("POST /new", handleCreate)
	mux.HandleFunc("POST /task/{id}/status", handleStatusChange)
	mux.HandleFunc("POST /task/{id}/comment", handleAddComment)
	mux.HandleFunc("POST /task/{id}/close", handleClose)
	mux.HandleFunc("POST /task/{id}/reopen", handleReopen)
	mux.HandleFunc("GET /task/{id}/edit", handleEditForm)
	mux.HandleFunc("POST /task/{id}/edit", handleEdit)
	mux.HandleFunc("GET /static/style.css", handleCSS)

	port := "9741"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}
	fmt.Printf("gig-controller running at http://localhost:%s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func handleCSS(w http.ResponseWriter, r *http.Request) {
	data, err := templateFS.ReadFile("templates/static/style.css")
	if err != nil {
		http.Error(w, "CSS not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/css")
	w.Write(data)
}
