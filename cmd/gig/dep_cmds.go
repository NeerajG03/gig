package main

import (
	"fmt"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func depCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dep",
		Short: "Manage task dependencies",
	}

	cmd.AddCommand(depAddCmd(), depRemoveCmd(), depListCmd(), depTreeCmd(), depCyclesCmd())
	return cmd
}

func depAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "add <task> <depends-on>",
		Short:             "Add dependency (task depends on depends-on)",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.AddDependency(args[0], args[1], gig.Blocks); err != nil {
				return err
			}
			fmt.Printf("%s now depends on %s\n", args[0], args[1])
			return nil
		},
	}
}

func depRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "remove <task> <depends-on>",
		Short:             "Remove a dependency",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.RemoveDependency(args[0], args[1]); err != nil {
				return err
			}
			fmt.Printf("Removed dependency: %s -> %s\n", args[0], args[1])
			return nil
		},
	}
}

func depListCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "list <id>",
		Short:             "Show dependencies for a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			deps, err := store.ListDependencies(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(deps)
			}
			if len(deps) == 0 {
				fmt.Println("No dependencies.")
				return nil
			}
			fmt.Printf("%s depends on:\n", args[0])
			for _, d := range deps {
				t, err := store.Get(d.ToID)
				if err == nil {
					fmt.Printf("  %s %s (%s)\n", d.ToID, t.Title, t.Status)
				} else {
					fmt.Printf("  %s (unknown)\n", d.ToID)
				}
			}
			return nil
		},
	}
}

func depTreeCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "tree <id>",
		Short:             "Show dependency tree",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			tree, err := store.DepTree(args[0])
			if err != nil {
				return err
			}
			fmt.Print(tree)
			return nil
		},
	}
}

func depCyclesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cycles",
		Short: "Detect circular dependencies",
		RunE: func(cmd *cobra.Command, args []string) error {
			cycles, err := store.DetectCycles()
			if err != nil {
				return err
			}
			if len(cycles) == 0 {
				fmt.Println("No cycles detected.")
				return nil
			}
			fmt.Printf("Found %d cycle(s):\n", len(cycles))
			for i, cycle := range cycles {
				fmt.Printf("  %d: %s\n", i+1, fmt.Sprintf("%v", cycle))
			}
			return nil
		},
	}
}
