// Package ui provides an embedded web-based kanban board for gig.
// Usage:
//
//	server := ui.New(store)
//	server.ListenAndServe(":9741")
package ui

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/NeerajG03/gig"
)

//go:embed templates/*
var templateFS embed.FS

// Column represents a kanban column for template rendering.
type Column struct {
	Status gig.Status
	Tasks  []*gig.Task
}

// BoardData is the template data for the board view.
type BoardData struct {
	Columns    []Column
	ReadyCount int
	TotalCount int
	View       string
}

// DetailData is the template data for the task detail view.
type DetailData struct {
	Task            *gig.Task
	Comments        []*gig.Comment
	DepsOn          []*DepInfo
	Blocks          []*DepInfo
	Children        []*gig.Task
	ChildrenColumns []Column
	Events          []*gig.Event
	AllTasks        []*gig.Task
}

// DepInfo pairs a dependency with its task info.
type DepInfo struct {
	Task *gig.Task
	Dep  *gig.Dependency
}

// Server is the gig web UI server.
type Server struct {
	store *gig.Store
	tmpl  *template.Template
	mux   *http.ServeMux
}

// New creates a new web UI server backed by the given store.
func New(store *gig.Store) *Server {
	s := &Server{store: store}
	s.parseTemplates()
	s.setupRoutes()
	return s
}

// ListenAndServe starts the HTTP server on the given address (e.g. ":9741").
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

// Handler returns the http.Handler for use with custom servers.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// CreateData is the template data for the create form.
type CreateData struct {
	AllTasks          []*gig.Task
	PreselectedParent string
}

var funcMap = template.FuncMap{
	"statusIcon": func(st gig.Status) string {
		switch st {
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
	"statusLabel": func(st gig.Status) string {
		switch st {
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
			return string(st)
		}
	},
	"truncate": func(s string, n int) string {
		if len(s) <= n {
			return s
		}
		return s[:n] + "..."
	},
	"columnColor": func(st gig.Status) string {
		switch st {
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

func (s *Server) parseTemplates() {
	s.tmpl = template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html", "templates/partials/*.html"),
	)
}

func (s *Server) setupRoutes() {
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("GET /", s.handleBoard)
	s.mux.HandleFunc("GET /task/{id}", s.handleDetail)
	s.mux.HandleFunc("GET /new", s.handleCreateForm)
	s.mux.HandleFunc("POST /new", s.handleCreate)
	s.mux.HandleFunc("POST /task/{id}/status", s.handleStatusChange)
	s.mux.HandleFunc("POST /task/{id}/comment", s.handleAddComment)
	s.mux.HandleFunc("POST /task/{id}/close", s.handleClose)
	s.mux.HandleFunc("POST /task/{id}/reopen", s.handleReopen)
	s.mux.HandleFunc("GET /task/{id}/edit", s.handleEditForm)
	s.mux.HandleFunc("POST /task/{id}/edit", s.handleEdit)
	s.mux.HandleFunc("GET /static/style.css", s.handleCSS)
}

func (s *Server) handleCSS(w http.ResponseWriter, r *http.Request) {
	data, err := templateFS.ReadFile("templates/static/style.css")
	if err != nil {
		http.Error(w, "CSS not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/css")
	w.Write(data)
}

var allStatuses = []gig.Status{
	gig.StatusOpen, gig.StatusInProgress, gig.StatusBlocked,
	gig.StatusDeferred, gig.StatusClosed,
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "top"
	}
	topOnly := view == "top"

	var columns []Column
	total := 0
	for _, st := range allStatuses {
		status := st
		tasks, _ := s.store.List(gig.ListParams{Status: &status})
		if topOnly {
			var filtered []*gig.Task
			for _, t := range tasks {
				if t.ParentID == "" {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}
		columns = append(columns, Column{Status: st, Tasks: tasks})
		total += len(tasks)
	}

	ready, _ := s.store.Ready("")

	data := BoardData{
		Columns:    columns,
		ReadyCount: len(ready),
		TotalCount: total,
		View:       view,
	}

	w.Header().Set("Content-Type", "text/html")
	s.tmpl.ExecuteTemplate(w, "board.html", data)
}

func (s *Server) handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := s.store.Get(id)
	if err != nil {
		http.Error(w, "Task not found", 404)
		return
	}

	comments, _ := s.store.ListComments(id)
	deps, _ := s.store.ListDependencies(id)
	dependents, _ := s.store.ListDependents(id)
	children, _ := s.store.Children(id)
	events, _ := s.store.Events(id)

	var depsOn []*DepInfo
	for _, d := range deps {
		t, err := s.store.Get(d.ToID)
		if err == nil {
			depsOn = append(depsOn, &DepInfo{Task: t, Dep: d})
		}
	}

	var blocks []*DepInfo
	for _, d := range dependents {
		t, err := s.store.Get(d.FromID)
		if err == nil {
			blocks = append(blocks, &DepInfo{Task: t, Dep: d})
		}
	}

	var childColumns []Column
	if len(children) > 0 {
		grouped := map[gig.Status][]*gig.Task{}
		for _, c := range children {
			grouped[c.Status] = append(grouped[c.Status], c)
		}
		for _, st := range allStatuses {
			if tasks, ok := grouped[st]; ok {
				childColumns = append(childColumns, Column{Status: st, Tasks: tasks})
			}
		}
	}

	data := DetailData{
		Task:            task,
		Comments:        comments,
		DepsOn:          depsOn,
		Blocks:          blocks,
		Children:        children,
		ChildrenColumns: childColumns,
		Events:          events,
	}

	w.Header().Set("Content-Type", "text/html")
	s.tmpl.ExecuteTemplate(w, "detail.html", data)
}

func (s *Server) handleCreateForm(w http.ResponseWriter, r *http.Request) {
	allTasks, _ := s.store.List(gig.ListParams{})
	data := CreateData{
		AllTasks:          allTasks,
		PreselectedParent: r.URL.Query().Get("parent"),
	}
	w.Header().Set("Content-Type", "text/html")
	s.tmpl.ExecuteTemplate(w, "create.html", data)
}

func parsePriority(val string) gig.Priority {
	switch val {
	case "0":
		return gig.P0
	case "1":
		return gig.P1
	case "3":
		return gig.P3
	case "4":
		return gig.P4
	default:
		return gig.P2
	}
}

func (s *Server) handleCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	var labels []string
	if l := r.FormValue("labels"); l != "" {
		labels = strings.Split(l, ",")
		for i := range labels {
			labels[i] = strings.TrimSpace(labels[i])
		}
	}

	parentID := r.FormValue("parent")
	_, err := s.store.Create(gig.CreateParams{
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		Type:        gig.TaskType(r.FormValue("type")),
		Priority:    parsePriority(r.FormValue("priority")),
		ParentID:    parentID,
		Assignee:    r.FormValue("assignee"),
		Labels:      labels,
		Notes:       r.FormValue("notes"),
		CreatedBy:   "web",
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if parentID != "" {
		http.Redirect(w, r, "/task/"+parentID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleStatusChange(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	newStatus := gig.Status(r.FormValue("status"))

	if err := s.store.UpdateStatus(id, newStatus, "web"); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/task/"+id)
		w.WriteHeader(200)
		return
	}
	http.Redirect(w, r, "/task/"+id, http.StatusSeeOther)
}

func (s *Server) handleAddComment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()

	author := r.FormValue("author")
	if author == "" {
		author = "web"
	}

	_, err := s.store.AddComment(id, author, r.FormValue("content"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		comments, _ := s.store.ListComments(id)
		s.tmpl.ExecuteTemplate(w, "comments.html", comments)
		return
	}
	http.Redirect(w, r, "/task/"+id, http.StatusSeeOther)
}

func (s *Server) handleClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	reason := r.FormValue("reason")

	if err := s.store.CloseTask(id, reason, "web"); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/task/"+id)
		w.WriteHeader(200)
		return
	}
	http.Redirect(w, r, "/task/"+id, http.StatusSeeOther)
}

func (s *Server) handleReopen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := s.store.Reopen(id, "web"); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/task/"+id)
		w.WriteHeader(200)
		return
	}
	http.Redirect(w, r, "/task/"+id, http.StatusSeeOther)
}

func (s *Server) handleEditForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := s.store.Get(id)
	if err != nil {
		http.Error(w, "Task not found", 404)
		return
	}

	allTasks, _ := s.store.List(gig.ListParams{})
	data := DetailData{Task: task, AllTasks: allTasks}
	w.Header().Set("Content-Type", "text/html")
	s.tmpl.ExecuteTemplate(w, "edit.html", data)
}

func (s *Server) handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()

	title := r.FormValue("title")
	desc := r.FormValue("description")
	assignee := r.FormValue("assignee")
	notes := r.FormValue("notes")
	priority := parsePriority(r.FormValue("priority"))

	var labels *[]string
	if l := r.FormValue("labels"); l != "" {
		parts := strings.Split(l, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		labels = &parts
	}

	_, err := s.store.Update(id, gig.UpdateParams{
		Title:       &title,
		Description: &desc,
		Priority:    &priority,
		Assignee:    &assignee,
		Notes:       &notes,
		Labels:      labels,
	}, "web")
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	http.Redirect(w, r, "/task/"+id, http.StatusSeeOther)
}
