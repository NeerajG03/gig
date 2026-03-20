package main

import (
	"fmt"

	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func searchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search tasks by title and description",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := store.Search(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(tasks)
			}
			if len(tasks) == 0 {
				fmt.Println("No matching tasks.")
				return nil
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
}

func readyCmd() *cobra.Command {
	var parentID string
	var showTree, showList bool

	cmd := &cobra.Command{
		Use:   "ready",
		Short: "Show open tasks available to pick up (no blockers)",
		RunE: func(cmd *cobra.Command, args []string) error {
			tasks, err := store.Ready(parentID)
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
			if quietOutput {
				for _, t := range tasks {
					fmt.Println(t.ID)
				}
				return nil
			}

			// Resolve view mode: flag > config > default("tree").
			// Ready defaults to tree (unlike list which defaults to list).
			viewMode := "tree"
			if cfg != nil && cfg.DefaultView != "" {
				viewMode = cfg.DefaultView
			}
			if showTree {
				viewMode = "tree"
			}
			if showList {
				viewMode = "list"
			}

			if viewMode == "tree" {
				printReadyTree(tasks)
			} else {
				printTaskTable(tasks)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&parentID, "id", "", "Scope to children of this task")
	cmd.Flags().BoolVar(&showTree, "tree", false, "Show as hierarchical tree (default)")
	cmd.Flags().BoolVar(&showList, "list", false, "Show as flat list")
	cmd.MarkFlagsMutuallyExclusive("tree", "list")
	cmd.RegisterFlagCompletionFunc("id", taskIDCompletion)
	return cmd
}

// printReadyTree groups ready tasks under their parent and renders as a tree.
// Parent tasks are fetched for context even if they are not themselves ready.
func printReadyTree(tasks []*gig.Task) {
	// Group by parent ID.
	type group struct {
		parent   *gig.Task   // nil for root-level ready tasks
		children []*gig.Task // ready tasks under this parent
	}

	groups := make(map[string]*group) // parent ID → group
	var rootTasks []*gig.Task         // ready tasks with no parent
	var groupOrder []string           // preserve order

	for _, t := range tasks {
		if t.ParentID == "" {
			rootTasks = append(rootTasks, t)
			continue
		}
		// Track that this parent has grouped children.
		// We'll skip it from root list later to avoid duplication.
		g, ok := groups[t.ParentID]
		if !ok {
			g = &group{}
			groups[t.ParentID] = g
			groupOrder = append(groupOrder, t.ParentID)
		}
		g.children = append(g.children, t)
	}

	// Fetch parent tasks for context.
	for pid, g := range groups {
		parent, err := store.Get(pid)
		if err == nil {
			g.parent = parent
		}
	}

	// Render grouped tasks first (parent → children).
	for _, pid := range groupOrder {
		g := groups[pid]
		if g.parent != nil {
			printTaskLine(g.parent)
		} else {
			fmt.Printf("%s (parent)\n", pid)
		}
		printSubtaskTree(g.children, "  ")
	}

	// Render root-level ready tasks (no parent), skipping those already shown as group parents.
	for _, t := range rootTasks {
		if _, shown := groups[t.ID]; shown {
			continue
		}
		printTaskLine(t)
	}

	printLegend()
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
}

func childrenCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "children <id>",
		Short:             "Show subtasks of a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
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
}
