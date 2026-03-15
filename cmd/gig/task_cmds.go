package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func createCmd() *cobra.Command {
	var desc, taskType, parentID, assignee, notes, labels string
	var priority int

	cmd := &cobra.Command{
		Use:   "create <title>",
		Short: "Create a new task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var labelList []string
			if labels != "" {
				labelList = strings.Split(labels, ",")
			}

			task, err := store.Create(gig.CreateParams{
				Title:       args[0],
				Description: desc,
				Type:        gig.TaskType(taskType),
				Priority:    gig.Priority(priority),
				ParentID:    parentID,
				Assignee:    assignee,
				Notes:       notes,
				Labels:      labelList,
			})
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(task)
			}
			if quietOutput {
				fmt.Println(task.ID)
				return nil
			}
			fmt.Printf("Created %s: %s\n", task.ID, task.Title)
			return nil
		},
	}

	cmd.Flags().StringVar(&desc, "desc", "", "Task description")
	cmd.Flags().StringVar(&taskType, "type", "task", "Task type (task|bug|feature|epic|chore)")
	cmd.Flags().IntVar(&priority, "priority", 2, "Priority (0=critical, 4=backlog)")
	cmd.Flags().StringVar(&parentID, "parent", "", "Parent task ID")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Assignee")
	cmd.Flags().StringVar(&notes, "notes", "", "Notes")
	cmd.Flags().StringVar(&labels, "labels", "", "Comma-separated labels")

	return cmd
}

func listCmd() *cobra.Command {
	var status, assignee, taskType, label, parentID string
	var priority, limit int
	var attrFilters []string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			params := gig.ListParams{
				Assignee: assignee,
				Label:    label,
				Limit:    limit,
			}
			if status != "" {
				s := gig.Status(status)
				params.Status = &s
			}
			if priority >= 0 {
				p := gig.Priority(priority)
				params.Priority = &p
			}
			if taskType != "" {
				t := gig.TaskType(taskType)
				params.Type = &t
			}
			if cmd.Flags().Changed("parent") {
				params.ParentID = &parentID
			}

			// Parse --attr key=value filters.
			if len(attrFilters) > 0 {
				params.AttrFilter = map[string]string{}
				for _, f := range attrFilters {
					parts := strings.SplitN(f, "=", 2)
					if len(parts) != 2 {
						return fmt.Errorf("invalid --attr format %q, expected key=value", f)
					}
					params.AttrFilter[parts[0]] = parts[1]
				}
			}

			tasks, err := store.List(params)
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(tasks)
			}

			if len(tasks) == 0 {
				fmt.Println("No tasks found.")
				return nil
			}

			for _, t := range tasks {
				if quietOutput {
					fmt.Println(t.ID)
					continue
				}
				printTaskLine(t)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&status, "status", "", "Filter by status")
	cmd.Flags().StringVar(&assignee, "assignee", "", "Filter by assignee")
	cmd.Flags().IntVar(&priority, "priority", -1, "Filter by priority")
	cmd.Flags().StringVar(&taskType, "type", "", "Filter by type")
	cmd.Flags().StringVar(&label, "label", "", "Filter by label")
	cmd.Flags().StringVar(&parentID, "parent", "", "Filter by parent ID")
	cmd.Flags().IntVar(&limit, "limit", 0, "Limit results")
	cmd.Flags().StringArrayVar(&attrFilters, "attr", nil, "Filter by attribute (key=value, repeatable)")

	return cmd
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show task details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := store.Get(args[0])
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(task)
			}

			fmt.Printf("ID:          %s\n", task.ID)
			fmt.Printf("Title:       %s\n", task.Title)
			fmt.Printf("Status:      %s\n", task.Status)
			fmt.Printf("Priority:    %s\n", task.Priority)
			fmt.Printf("Type:        %s\n", task.Type)
			if task.Assignee != "" {
				fmt.Printf("Assignee:    %s\n", task.Assignee)
			}
			if task.ParentID != "" {
				fmt.Printf("Parent:      %s\n", task.ParentID)
			}
			if task.Description != "" {
				fmt.Printf("Description: %s\n", task.Description)
			}
			if task.Notes != "" {
				fmt.Printf("Notes:       %s\n", task.Notes)
			}
			if len(task.Labels) > 0 {
				fmt.Printf("Labels:      %s\n", strings.Join(task.Labels, ", "))
			}
			fmt.Printf("Created:     %s\n", task.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("Updated:     %s\n", task.UpdatedAt.Format("2006-01-02 15:04"))
			if task.ClosedAt != nil {
				fmt.Printf("Closed:      %s\n", task.ClosedAt.Format("2006-01-02 15:04"))
			}
			if task.CloseReason != "" {
				fmt.Printf("Reason:      %s\n", task.CloseReason)
			}

			// Show comments.
			comments, _ := store.ListComments(task.ID)
			if len(comments) > 0 {
				fmt.Printf("\nComments (%d):\n", len(comments))
				for _, c := range comments {
					author := c.Author
					if author == "" {
						author = "anonymous"
					}
					fmt.Printf("  [%s] %s: %s\n", c.CreatedAt.Format("01-02 15:04"), author, c.Content)
				}
			}

			// Show dependencies.
			deps, _ := store.ListDependencies(task.ID)
			if len(deps) > 0 {
				fmt.Printf("\nDepends on:\n")
				for _, d := range deps {
					depTask, err := store.Get(d.ToID)
					if err == nil {
						fmt.Printf("  %s %s (%s)\n", d.ToID, depTask.Title, depTask.Status)
					}
				}
			}

			dependents, _ := store.ListDependents(task.ID)
			if len(dependents) > 0 {
				fmt.Printf("\nBlocks:\n")
				for _, d := range dependents {
					depTask, err := store.Get(d.FromID)
					if err == nil {
						fmt.Printf("  %s %s (%s)\n", d.FromID, depTask.Title, depTask.Status)
					}
				}
			}

			// Show children.
			children, _ := store.Children(task.ID)
			if len(children) > 0 {
				fmt.Printf("\nChildren (%d):\n", len(children))
				for _, c := range children {
					fmt.Printf("  ")
					printTaskLine(c)
				}
			}

			// Show custom attributes.
			attrs, _ := store.Attrs(task.ID)
			if len(attrs) > 0 {
				fmt.Printf("\nAttributes:\n")
				for _, a := range attrs {
					fmt.Printf("  %s = %s (%s)\n", a.Key, a.Value, a.Type)
				}
			}

			return nil
		},
	}
}

func updateCmd() *cobra.Command {
	var title, desc, status, assignee, notes, labels string
	var priority int
	var claim bool

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			if claim {
				if assignee == "" {
					assignee = "cli"
				}
				if err := store.Claim(id, assignee); err != nil {
					return err
				}
				fmt.Printf("Claimed %s by %s\n", id, assignee)
				return nil
			}

			if status != "" {
				if err := store.UpdateStatus(id, gig.Status(status), "cli"); err != nil {
					return err
				}
				fmt.Printf("Status of %s set to %s\n", id, status)
				return nil
			}

			params := gig.UpdateParams{}
			if cmd.Flags().Changed("title") {
				params.Title = &title
			}
			if cmd.Flags().Changed("desc") {
				params.Description = &desc
			}
			if cmd.Flags().Changed("priority") {
				p := gig.Priority(priority)
				params.Priority = &p
			}
			if cmd.Flags().Changed("assignee") {
				params.Assignee = &assignee
			}
			if cmd.Flags().Changed("notes") {
				params.Notes = &notes
			}
			if cmd.Flags().Changed("labels") {
				l := strings.Split(labels, ",")
				params.Labels = &l
			}

			task, err := store.Update(id, params, "cli")
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(task)
			}
			fmt.Printf("Updated %s: %s\n", task.ID, task.Title)
			return nil
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "New title")
	cmd.Flags().StringVar(&desc, "desc", "", "New description")
	cmd.Flags().StringVar(&status, "status", "", "New status")
	cmd.Flags().IntVar(&priority, "priority", -1, "New priority")
	cmd.Flags().StringVar(&assignee, "assignee", "", "New assignee")
	cmd.Flags().StringVar(&notes, "notes", "", "New notes")
	cmd.Flags().StringVar(&labels, "labels", "", "New labels (comma-separated)")
	cmd.Flags().BoolVar(&claim, "claim", false, "Claim task (set assignee + in_progress)")

	return cmd
}

func closeCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:   "close <id> [id2...]",
		Short: "Close one or more tasks",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := store.CloseTask(id, reason, "cli"); err != nil {
					return err
				}
				if !quietOutput {
					fmt.Printf("Closed %s\n", id)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Close reason")
	return cmd
}

func reopenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reopen <id>",
		Short: "Reopen a closed task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.Reopen(args[0], "cli"); err != nil {
				return err
			}
			fmt.Printf("Reopened %s\n", args[0])
			return nil
		},
	}
}

func readyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ready",
		Short: "Show tasks with no blockers",
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := store.Ready()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(tasks)
			}
			if len(tasks) == 0 {
				fmt.Println("No ready tasks.")
				return nil
			}
			for _, t := range tasks {
				if quietOutput {
					fmt.Println(t.ID)
					continue
				}
				printTaskLine(t)
			}
			return nil
		},
	}
}

func blockedCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "blocked",
		Short: "Show blocked tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := store.Blocked()
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(tasks)
			}
			if len(tasks) == 0 {
				fmt.Println("No blocked tasks.")
				return nil
			}
			for _, t := range tasks {
				if quietOutput {
					fmt.Println(t.ID)
					continue
				}
				printTaskLine(t)
			}
			return nil
		},
	}
}

func childrenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "children <id>",
		Short: "Show subtasks of a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := store.Children(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(tasks)
			}
			if len(tasks) == 0 {
				fmt.Println("No children.")
				return nil
			}
			for _, t := range tasks {
				if quietOutput {
					fmt.Println(t.ID)
					continue
				}
				printTaskLine(t)
			}
			return nil
		},
	}
}

// printTaskLine prints a single-line summary of a task.
func printTaskLine(t *gig.Task) {
	icon := statusIcon(t.Status)
	assignee := ""
	if t.Assignee != "" {
		assignee = fmt.Sprintf(" @%s", t.Assignee)
	}
	fmt.Printf("%s [%s] P%d %s%s\n", t.ID, icon, t.Priority, t.Title, assignee)
}

func statusIcon(s gig.Status) string {
	switch s {
	case gig.StatusOpen:
		return " "
	case gig.StatusInProgress:
		return ">"
	case gig.StatusBlocked:
		return "!"
	case gig.StatusDeferred:
		return "~"
	case gig.StatusClosed:
		return "x"
	default:
		return "?"
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
