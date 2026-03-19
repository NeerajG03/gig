package main

import (
	"fmt"
	"strings"

	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func checkpointCmd() *cobra.Command {
	var (
		done      string
		decisions string
		next      string
		blockers  string
		files     []string
		author    string
	)

	cmd := &cobra.Command{
		Use:               "checkpoint <id>",
		Short:             "Add a structured progress snapshot to a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cp, err := store.AddCheckpoint(args[0], author, gig.CheckpointParams{
				Done:      done,
				Decisions: decisions,
				Next:      next,
				Blockers:  blockers,
				Files:     files,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(cp)
			}
			fmt.Printf("Checkpoint added to %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&done, "done", "", "What was accomplished (required)")
	cmd.Flags().StringVar(&decisions, "decisions", "", "Key decisions and reasoning")
	cmd.Flags().StringVar(&next, "next", "", "What should happen next")
	cmd.Flags().StringVar(&blockers, "blockers", "", "Current blockers")
	cmd.Flags().StringSliceVar(&files, "files", nil, "File paths touched or referenced")
	cmd.Flags().StringVar(&author, "author", "", "Checkpoint author")
	cmd.MarkFlagRequired("done")
	return cmd
}

func checkpointsCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "checkpoints <id>",
		Short:             "List checkpoints on a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			cps, err := store.ListCheckpoints(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(cps)
			}
			if len(cps) == 0 {
				fmt.Println("No checkpoints.")
				return nil
			}
			for i, cp := range cps {
				if i > 0 {
					fmt.Println("---")
				}
				author := cp.Author
				if author == "" {
					author = "anonymous"
				}
				fmt.Printf("[%s] %s\n", cp.CreatedAt.Format("2006-01-02 15:04"), author)
				fmt.Printf("  Done:      %s\n", cp.Done)
				if cp.Decisions != "" {
					fmt.Printf("  Decisions: %s\n", cp.Decisions)
				}
				if cp.Next != "" {
					fmt.Printf("  Next:      %s\n", cp.Next)
				}
				if cp.Blockers != "" {
					fmt.Printf("  Blockers:  %s\n", cp.Blockers)
				}
				if len(cp.Files) > 0 {
					fmt.Printf("  Files:     %s\n", strings.Join(cp.Files, ", "))
				}
			}
			return nil
		},
	}
}
