package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/NeerajG03/gig"
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
				CreatedBy:   actorName,
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

	_ = cmd.RegisterFlagCompletionFunc("type", taskTypeCompletion)
	_ = cmd.RegisterFlagCompletionFunc("priority", priorityCompletion)
	_ = cmd.RegisterFlagCompletionFunc("parent", taskIDCompletion)

	return cmd
}

func showCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show <id>",
		Short:             "Show task details",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			task, err := store.GetFull(args[0])
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(task)
			}

			fmt.Printf("%s  %s\n", colorize(dim, "ID:"), colorID(task.ID))
			fmt.Printf("%s   %s\n", colorize(dim, "Title:"), task.Title)
			fmt.Printf("%s  %s %s\n", colorize(dim, "Status:"), colorStatus(task.Status), string(task.Status))
			fmt.Printf("%s %s\n", colorize(dim, "Priority:"), colorPriority(task.Priority)+" "+colorize(dim, priorityLabel(task.Priority)))
			fmt.Printf("%s    %s\n", colorize(dim, "Type:"), task.Type)
			if task.Assignee != "" {
				fmt.Printf("%s %s\n", colorize(dim, "Assignee:"), colorAssignee(task.Assignee))
			}
			if task.ParentID != "" {
				fmt.Printf("%s  %s\n", colorize(dim, "Parent:"), colorID(task.ParentID))
			}
			if task.Description != "" {
				fmt.Printf("%s    %s\n", colorize(dim, "Desc:"), task.Description)
			}
			if task.Notes != "" {
				fmt.Printf("%s   %s\n", colorize(dim, "Notes:"), task.Notes)
			}
			if len(task.Labels) > 0 {
				fmt.Printf("%s  %s\n", colorize(dim, "Labels:"), strings.Join(task.Labels, ", "))
			}
			fmt.Printf("%s %s\n", colorize(dim, "Created:"), task.CreatedAt.Format("2006-01-02 15:04"))
			fmt.Printf("%s %s\n", colorize(dim, "Updated:"), task.UpdatedAt.Format("2006-01-02 15:04"))
			if task.ClosedAt != nil {
				fmt.Printf("%s  %s\n", colorize(dim, "Closed:"), task.ClosedAt.Format("2006-01-02 15:04"))
			}
			if task.CloseReason != "" {
				fmt.Printf("%s  %s\n", colorize(dim, "Reason:"), task.CloseReason)
			}

			comments, _ := store.ListComments(task.ID)
			if len(comments) > 0 {
				fmt.Printf("\n%s (%d):\n", colorize(dim, "Comments"), len(comments))
				for _, c := range comments {
					author := c.Author
					if author == "" {
						author = "anonymous"
					}
					fmt.Printf("  %s %s: %s\n", colorize(dim, "["+c.CreatedAt.Format("01-02 15:04")+"]"), colorize(cyan, author), c.Content)
				}
			}

			cp, _ := store.LatestCheckpoint(task.ID)
			if cp != nil {
				fmt.Printf("\n%s %s\n", colorize(dim, "Latest checkpoint:"), colorize(dim, "["+cp.CreatedAt.Format("2006-01-02 15:04")+"]"))
				fmt.Printf("  %s %s\n", colorize(dim, "Done:"), cp.Done)
				if cp.Decisions != "" {
					fmt.Printf("  %s %s\n", colorize(dim, "Decisions:"), cp.Decisions)
				}
				if cp.Next != "" {
					fmt.Printf("  %s %s\n", colorize(dim, "Next:"), cp.Next)
				}
				if cp.Blockers != "" {
					fmt.Printf("  %s %s\n", colorize(dim, "Blockers:"), cp.Blockers)
				}
				if len(cp.Files) > 0 {
					fmt.Printf("  %s %s\n", colorize(dim, "Files:"), strings.Join(cp.Files, ", "))
				}
			}

			deps, _ := store.ListDependencies(task.ID)
			if len(deps) > 0 {
				fmt.Printf("\n%s\n", colorize(dim, "Depends on:"))
				for _, d := range deps {
					depTask, err := store.Get(d.ToID)
					if err == nil {
						fmt.Printf("  %s %s %s\n", colorID(d.ToID), depTask.Title, colorStatus(depTask.Status))
					}
				}
			}

			dependents, _ := store.ListDependents(task.ID)
			if len(dependents) > 0 {
				fmt.Printf("\n%s\n", colorize(dim, "Blocks:"))
				for _, d := range dependents {
					depTask, err := store.Get(d.FromID)
					if err == nil {
						fmt.Printf("  %s %s %s\n", colorID(d.FromID), depTask.Title, colorStatus(depTask.Status))
					}
				}
			}

			tree, _ := store.GetTree(task.ID)
			if tree != nil && len(tree.Children) > 0 {
				fmt.Printf("\n%s (%d):\n", colorize(dim, "Subtasks"), countDescendants(tree))
				printSubtaskTree(tree.Children, "  ")
			}

			attrs, _ := store.Attrs(task.ID)
			if len(attrs) > 0 {
				fmt.Printf("\n%s\n", colorize(dim, "Attributes:"))
				for _, a := range attrs {
					fmt.Printf("  %s = %s %s\n", a.Key, a.Value, colorize(dim, "("+string(a.Type)+")"))
				}
			}

			return nil
		},
	}
}

func updateCmd() *cobra.Command {
	var title, desc, status, assignee, notes, labels, parent string
	var priority int
	var claim, orphan bool

	cmd := &cobra.Command{
		Use:               "update <id>",
		Short:             "Update a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: openTaskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]

			if claim {
				if assignee == "" {
					assignee = actorName
				}
				result, err := store.Claim(id, assignee)
				if err != nil {
					return err
				}
				fmt.Printf("Claimed %s by %s\n", id, assignee)
				if result.ParentProgressed {
					fmt.Printf("Parent %s → in_progress\n", result.ParentID)
				}
				return nil
			}

			if status != "" {
				// Check parent before to detect auto-progress.
				task, _ := store.Get(id)
				var parentStatusBefore gig.Status
				if task != nil && task.ParentID != "" {
					if p, err := store.Get(task.ParentID); err == nil {
						parentStatusBefore = p.Status
					}
				}

				if err := store.UpdateStatus(id, gig.Status(status), actorName); err != nil {
					return err
				}
				fmt.Printf("Status of %s set to %s\n", id, status)

				// Report parent auto-progress.
				if task != nil && task.ParentID != "" && parentStatusBefore == gig.StatusOpen {
					if p, err := store.Get(task.ParentID); err == nil && p.Status == gig.StatusInProgress {
						fmt.Printf("Parent %s → in_progress\n", task.ParentID)
					}
				}
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
			if cmd.Flags().Changed("parent") {
				params.ParentID = &parent
			}
			if orphan {
				params.Orphan = true
			}

			task, err := store.Update(id, params, actorName)
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
	cmd.Flags().StringVar(&parent, "parent", "", "Set parent task ID")
	cmd.Flags().BoolVar(&orphan, "orphan", false, "Remove parent (make top-level)")
	cmd.Flags().BoolVar(&claim, "claim", false, "Claim task (set assignee + in_progress)")
	cmd.MarkFlagsMutuallyExclusive("parent", "orphan")

	_ = cmd.RegisterFlagCompletionFunc("status", statusCompletion)
	_ = cmd.RegisterFlagCompletionFunc("priority", priorityCompletion)

	return cmd
}

func closeCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:               "close <id> [id2...]",
		Short:             "Close one or more tasks",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: openTaskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := store.CloseTask(id, reason, actorName); err != nil {
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

func cancelCmd() *cobra.Command {
	var reason string

	cmd := &cobra.Command{
		Use:               "cancel <id> [id2...]",
		Short:             "Cancel one or more tasks",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: openTaskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := store.CancelTask(id, reason, actorName); err != nil {
					return err
				}
				if !quietOutput {
					fmt.Printf("Cancelled %s\n", id)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&reason, "reason", "", "Cancellation reason")
	return cmd
}

func deleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "delete <id> [id2...]",
		Short:             "Permanently delete one or more tasks (and their children)",
		Args:              cobra.MinimumNArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			for _, id := range args {
				if err := store.DeleteTask(id, actorName); err != nil {
					return err
				}
				if !quietOutput {
					fmt.Printf("Deleted %s\n", id)
				}
			}
			return nil
		},
	}
}

func reopenCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "reopen <id>",
		Short:             "Reopen a closed task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: closedTaskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.Reopen(args[0], actorName); err != nil {
				return err
			}
			fmt.Printf("Reopened %s\n", args[0])
			return nil
		},
	}
}

func printTaskLine(t *gig.Task) {
	assignee := ""
	if t.Assignee != "" {
		assignee = " " + colorAssignee(t.Assignee)
	}
	fmt.Printf("%s %s %s %s%s\n", colorID(t.ID), colorStatus(t.Status), colorPriority(t.Priority), t.Title, assignee)
}

func printTaskTable(tasks []*gig.Task) {
	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return
	}

	maxID := 0
	for _, t := range tasks {
		if len(t.ID) > maxID {
			maxID = len(t.ID)
		}
	}

	for _, t := range tasks {
		assignee := ""
		if t.Assignee != "" {
			assignee = " " + colorAssignee(t.Assignee)
		}
		padded := fmt.Sprintf("%-*s", maxID, t.ID)
		fmt.Printf("%s %s %s %s%s\n", colorize(dim, padded), colorStatus(t.Status), colorPriority(t.Priority), t.Title, assignee)
	}
	printLegend()
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
	case gig.StatusCancelled:
		return "-"
	default:
		return "?"
	}
}

func priorityLabel(p gig.Priority) string {
	switch p {
	case gig.P0:
		return "(critical)"
	case gig.P1:
		return "(high)"
	case gig.P2:
		return "(medium)"
	case gig.P3:
		return "(low)"
	case gig.P4:
		return "(backlog)"
	default:
		return ""
	}
}

func printSubtaskTree(tasks []*gig.Task, indent string) {
	for i, t := range tasks {
		connector := "├── "
		childIndent := indent + "│   "
		if i == len(tasks)-1 {
			connector = "└── "
			childIndent = indent + "    "
		}
		assignee := ""
		if t.Assignee != "" {
			assignee = " " + colorAssignee(t.Assignee)
		}
		fmt.Printf("%s%s%s %s %s%s\n", indent, connector, colorStatus(t.Status), colorID(t.ID), t.Title, assignee)
		if len(t.Children) > 0 {
			printSubtaskTree(t.Children, childIndent)
		}
	}
}

func countDescendants(t *gig.Task) int {
	count := len(t.Children)
	for _, c := range t.Children {
		count += countDescendants(c)
	}
	return count
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
