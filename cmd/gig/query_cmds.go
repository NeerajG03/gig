package main

import (
	"fmt"

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
