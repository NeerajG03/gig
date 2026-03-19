package main

import (
	"net/http"
	"strings"

	"github.com/NeerajG03/gig"
)

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
	View       string // "top" or "all"
}

// DetailData is the template data for the task detail view.
type DetailData struct {
	Task            *gig.Task
	Comments        []*gig.Comment
	DepsOn          []*DepInfo
	Blocks          []*DepInfo
	Children        []*gig.Task
	ChildrenColumns []Column // children grouped by status for mini kanban
	Events          []*gig.Event
	AllTasks        []*gig.Task // for parent selector in forms
}

// DepInfo pairs a dependency with its task info.
type DepInfo struct {
	Task *gig.Task
	Dep  *gig.Dependency
}

func handleBoard(w http.ResponseWriter, r *http.Request) {
	statuses := []gig.Status{
		gig.StatusOpen, gig.StatusInProgress, gig.StatusBlocked,
		gig.StatusDeferred, gig.StatusClosed,
	}

	// Filter: "top" (default) shows only root tasks, "all" shows everything.
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "top"
	}
	topOnly := view == "top"

	var columns []Column
	total := 0
	for _, s := range statuses {
		status := s
		tasks, _ := store.List(gig.ListParams{Status: &status})
		if topOnly {
			var filtered []*gig.Task
			for _, t := range tasks {
				if t.ParentID == "" {
					filtered = append(filtered, t)
				}
			}
			tasks = filtered
		}
		columns = append(columns, Column{Status: s, Tasks: tasks})
		total += len(tasks)
	}

	ready, _ := store.Ready("")

	data := BoardData{
		Columns:    columns,
		ReadyCount: len(ready),
		TotalCount: total,
		View:       view,
	}

	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "board.html", data)
}

func handleDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := store.Get(id)
	if err != nil {
		http.Error(w, "Task not found", 404)
		return
	}

	comments, _ := store.ListComments(id)
	deps, _ := store.ListDependencies(id)
	dependents, _ := store.ListDependents(id)
	children, _ := store.Children(id)
	events, _ := store.Events(id)

	var depsOn []*DepInfo
	for _, d := range deps {
		t, err := store.Get(d.ToID)
		if err == nil {
			depsOn = append(depsOn, &DepInfo{Task: t, Dep: d})
		}
	}

	var blocks []*DepInfo
	for _, d := range dependents {
		t, err := store.Get(d.FromID)
		if err == nil {
			blocks = append(blocks, &DepInfo{Task: t, Dep: d})
		}
	}

	// Group children by status for mini kanban.
	var childColumns []Column
	if len(children) > 0 {
		statusOrder := []gig.Status{
			gig.StatusOpen, gig.StatusInProgress, gig.StatusBlocked,
			gig.StatusDeferred, gig.StatusClosed,
		}
		grouped := map[gig.Status][]*gig.Task{}
		for _, c := range children {
			grouped[c.Status] = append(grouped[c.Status], c)
		}
		for _, s := range statusOrder {
			if tasks, ok := grouped[s]; ok {
				childColumns = append(childColumns, Column{Status: s, Tasks: tasks})
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
	tmpl.ExecuteTemplate(w, "detail.html", data)
}

func handleCreateForm(w http.ResponseWriter, r *http.Request) {
	allTasks, _ := store.List(gig.ListParams{})
	data := DetailData{AllTasks: allTasks}
	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "create.html", data)
}

func handleCreate(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	priority := gig.P2
	switch r.FormValue("priority") {
	case "0":
		priority = gig.P0
	case "1":
		priority = gig.P1
	case "3":
		priority = gig.P3
	case "4":
		priority = gig.P4
	}

	var labels []string
	if l := r.FormValue("labels"); l != "" {
		labels = strings.Split(l, ",")
		for i := range labels {
			labels[i] = strings.TrimSpace(labels[i])
		}
	}

	_, err := store.Create(gig.CreateParams{
		Title:       r.FormValue("title"),
		Description: r.FormValue("description"),
		Type:        gig.TaskType(r.FormValue("type")),
		Priority:    priority,
		ParentID:    r.FormValue("parent"),
		Assignee:    r.FormValue("assignee"),
		Labels:      labels,
		Notes:       r.FormValue("notes"),
		CreatedBy:   "web",
	})
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleStatusChange(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	newStatus := gig.Status(r.FormValue("status"))

	if err := store.UpdateStatus(id, newStatus, "web"); err != nil {
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

func handleAddComment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()

	author := r.FormValue("author")
	if author == "" {
		author = "web"
	}

	_, err := store.AddComment(id, author, r.FormValue("content"))
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// If HTMX request, re-render the comments section.
	if r.Header.Get("HX-Request") == "true" {
		comments, _ := store.ListComments(id)
		tmpl.ExecuteTemplate(w, "comments.html", comments)
		return
	}
	http.Redirect(w, r, "/task/"+id, http.StatusSeeOther)
}

func handleClose(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()
	reason := r.FormValue("reason")

	if err := store.CloseTask(id, reason, "web"); err != nil {
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

func handleReopen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if err := store.Reopen(id, "web"); err != nil {
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

func handleEditForm(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	task, err := store.Get(id)
	if err != nil {
		http.Error(w, "Task not found", 404)
		return
	}

	allTasks, _ := store.List(gig.ListParams{})
	data := DetailData{Task: task, AllTasks: allTasks}
	w.Header().Set("Content-Type", "text/html")
	tmpl.ExecuteTemplate(w, "edit.html", data)
}

func handleEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	r.ParseForm()

	title := r.FormValue("title")
	desc := r.FormValue("description")
	assignee := r.FormValue("assignee")
	notes := r.FormValue("notes")

	priority := gig.P2
	switch r.FormValue("priority") {
	case "0":
		priority = gig.P0
	case "1":
		priority = gig.P1
	case "3":
		priority = gig.P3
	case "4":
		priority = gig.P4
	}

	var labels *[]string
	if l := r.FormValue("labels"); l != "" {
		parts := strings.Split(l, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		labels = &parts
	}

	_, err := store.Update(id, gig.UpdateParams{
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
