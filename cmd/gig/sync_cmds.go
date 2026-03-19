package main

import (
	"fmt"
	"path/filepath"

	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

func exportCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export tasks to JSONL",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				file = filepath.Join(gig.DefaultGigHome(), "tasks.jsonl")
			}
			if err := store.ExportJSONL(file); err != nil {
				return err
			}
			fmt.Printf("Exported tasks to %s\n", file)
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Output file path")
	return cmd
}

func importCmd() *cobra.Command {
	var file string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import tasks from JSONL",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				file = filepath.Join(gig.DefaultGigHome(), "tasks.jsonl")
			}
			if err := store.ImportJSONL(file); err != nil {
				return err
			}
			fmt.Printf("Imported tasks from %s\n", file)
			return nil
		},
	}

	cmd.Flags().StringVar(&file, "file", "", "Input file path")
	return cmd
}

func syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Export tasks and events for backup/sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			home := gig.DefaultGigHome()

			tasksPath := filepath.Join(home, "tasks.jsonl")
			if err := store.ExportJSONL(tasksPath); err != nil {
				return fmt.Errorf("export tasks: %w", err)
			}
			fmt.Printf("Exported tasks to %s\n", tasksPath)

			eventsPath := filepath.Join(home, "events.jsonl")
			if err := store.ExportEvents(eventsPath); err != nil {
				return fmt.Errorf("export events: %w", err)
			}
			fmt.Printf("Exported events to %s\n", eventsPath)

			return nil
		},
	}
}
