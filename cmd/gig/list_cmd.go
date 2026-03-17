package main

import (
	"fmt"
	"strings"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func listCmd() *cobra.Command {
	var status, assignee, taskType, label, parentID string
	var priority, limit int
	var attrFilters []string
	var showTree, showList, showAll bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve view mode: flag > config > default("list").
			viewMode := "list"
			if cfg != nil && cfg.DefaultView == "tree" {
				viewMode = "tree"
			}
			if showTree {
				viewMode = "tree"
			}
			if showList {
				viewMode = "list"
			}

			// Resolve show_all: flag > config > default(false).
			includeAll := false
			if cfg != nil && cfg.ShowAll {
				includeAll = true
			}
			if showAll {
				includeAll = true
			}

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

			// Exclude closed tasks by default (unless --all or explicit --status).
			if !includeAll && !cmd.Flags().Changed("status") {
				params.ExcludeStatuses = []gig.Status{gig.StatusClosed}
			}

			if viewMode == "tree" {
				return listTree(cmd, params, includeAll)
			}

			tasks, err := store.List(params)
			if err != nil {
				return err
			}

			if jsonOutput {
				return printJSON(tasks)
			}

			if quietOutput {
				for _, t := range tasks {
					fmt.Println(t.ID)
				}
				return nil
			}
			printTaskTable(tasks)
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
	cmd.Flags().BoolVar(&showTree, "tree", false, "Show tasks as hierarchical tree")
	cmd.Flags().BoolVar(&showList, "list", false, "Show tasks as flat list (default)")
	cmd.Flags().BoolVarP(&showAll, "all", "a", false, "Include closed tasks")
	cmd.MarkFlagsMutuallyExclusive("tree", "list")

	_ = cmd.RegisterFlagCompletionFunc("status", statusCompletion)
	_ = cmd.RegisterFlagCompletionFunc("type", taskTypeCompletion)
	_ = cmd.RegisterFlagCompletionFunc("priority", priorityCompletion)
	_ = cmd.RegisterFlagCompletionFunc("parent", taskIDCompletion)

	return cmd
}

// listTree renders tasks as a hierarchical tree.
func listTree(cmd *cobra.Command, params gig.ListParams, includeAll bool) error {
	// For tree view, fetch all root tasks (including closed) so we can keep
	// closed parents that have open children. Filtering happens in filterTree.
	if !cmd.Flags().Changed("parent") {
		rootID := ""
		params.ParentID = &rootID
	}
	params.ExcludeStatuses = nil

	// Save and clear the status filter — we need all roots so we can find
	// matching subtasks deep in the tree. Inclusion filtering happens after
	// building full trees.
	var includeStatus *gig.Status
	if params.Status != nil {
		includeStatus = params.Status
		params.Status = nil
	}

	tasks, err := store.List(params)
	if err != nil {
		return err
	}

	buildTrees := func(tasks []*gig.Task) ([]*gig.Task, error) {
		var trees []*gig.Task
		for _, t := range tasks {
			tree, err := store.GetTree(t.ID)
			if err != nil {
				return nil, err
			}
			trees = append(trees, tree)
		}
		if includeStatus != nil {
			trees = filterTreeInclude(trees, *includeStatus)
		}
		if !includeAll {
			trees = filterTree(trees, []gig.Status{gig.StatusClosed})
		}
		return trees, nil
	}

	if jsonOutput {
		trees, err := buildTrees(tasks)
		if err != nil {
			return err
		}
		return printJSON(trees)
	}

	if quietOutput {
		for _, t := range tasks {
			fmt.Println(t.ID)
		}
		return nil
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	trees, err := buildTrees(tasks)
	if err != nil {
		return err
	}
	for _, tree := range trees {
		printTaskLine(tree)
		if len(tree.Children) > 0 {
			printSubtaskTree(tree.Children, "  ")
		}
	}
	return nil
}

// filterTreeInclude keeps only trees where at least one node matches the
// given status. Ancestor nodes are preserved so the tree structure is intact.
func filterTreeInclude(tasks []*gig.Task, status gig.Status) []*gig.Task {
	var result []*gig.Task
	for _, t := range tasks {
		t.Children = filterTreeInclude(t.Children, status)
		if t.Status == status || len(t.Children) > 0 {
			result = append(result, t)
		}
	}
	return result
}

// filterTree recursively removes tasks with excluded statuses from the tree.
// A task is kept if it has the excluded status but still has visible children.
func filterTree(tasks []*gig.Task, exclude []gig.Status) []*gig.Task {
	var result []*gig.Task
	for _, t := range tasks {
		t.Children = filterTree(t.Children, exclude)

		excluded := false
		for _, s := range exclude {
			if t.Status == s {
				excluded = true
				break
			}
		}
		if !excluded || len(t.Children) > 0 {
			result = append(result, t)
		}
	}
	return result
}
