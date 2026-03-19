package main

import (
	"fmt"
	"os"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var prefix string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize gig (creates ~/.gig/ with config and database)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home := gig.DefaultGigHome()

			configPath := gig.DefaultConfigPath()
			if _, err := os.Stat(configPath); err == nil {
				fmt.Printf("gig already initialized at %s\n", home)
				return nil
			}

			cfg := gig.DefaultConfig()
			if prefix != "" {
				cfg.Prefix = prefix
			}

			if err := gig.SaveConfig("", &cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			s, err := gig.Open(cfg.DBPath, gig.WithPrefix(cfg.Prefix))
			if err != nil {
				return fmt.Errorf("init database: %w", err)
			}
			s.Close()

			fmt.Printf("Initialized gig at %s\n", home)
			fmt.Printf("  Config: %s\n", configPath)
			fmt.Printf("  Database: %s\n", cfg.DBPath)
			fmt.Printf("  Prefix: %s\n", cfg.Prefix)
			return nil
		},
	}

	cmd.Flags().StringVar(&prefix, "prefix", "", "ID prefix (default: gig)")
	return cmd
}

func eventsCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "events <id>",
		Short:             "Show event history for a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			events, err := store.Events(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(events)
			}
			if len(events) == 0 {
				fmt.Println("No events.")
				return nil
			}
			for _, e := range events {
				actor := e.Actor
				if actor == "" {
					actor = "system"
				}
				switch e.Type {
				case gig.EventStatusChanged:
					fmt.Printf("[%s] %s: status %s -> %s (%s)\n",
						e.Timestamp.Format("01-02 15:04"), actor, e.OldValue, e.NewValue, e.Type)
				case gig.EventAssigned:
					fmt.Printf("[%s] %s: assigned to %s\n",
						e.Timestamp.Format("01-02 15:04"), actor, e.NewValue)
				case gig.EventCommented:
					fmt.Printf("[%s] %s: commented\n",
						e.Timestamp.Format("01-02 15:04"), actor)
				default:
					detail := ""
					if e.Field != "" {
						detail = fmt.Sprintf(" (%s: %s -> %s)", e.Field, e.OldValue, e.NewValue)
					}
					fmt.Printf("[%s] %s: %s%s\n",
						e.Timestamp.Format("01-02 15:04"), actor, e.Type, detail)
				}
			}
			return nil
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show task statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := store.List(gig.ListParams{})
			ready, _ := store.Ready("")
			blocked, _ := store.Blocked()

			counts := map[gig.Status]int{}
			for _, t := range all {
				counts[t.Status]++
			}

			if jsonOutput {
				return printJSON(map[string]any{
					"total":       len(all),
					"open":        counts[gig.StatusOpen],
					"in_progress": counts[gig.StatusInProgress],
					"blocked":     len(blocked),
					"deferred":    counts[gig.StatusDeferred],
					"closed":      counts[gig.StatusClosed],
					"ready":       len(ready),
				})
			}

			fmt.Printf("Total:       %d\n", len(all))
			fmt.Printf("Open:        %d\n", counts[gig.StatusOpen])
			fmt.Printf("In Progress: %d\n", counts[gig.StatusInProgress])
			fmt.Printf("Blocked:     %d\n", len(blocked))
			fmt.Printf("Deferred:    %d\n", counts[gig.StatusDeferred])
			fmt.Printf("Closed:      %d\n", counts[gig.StatusClosed])
			fmt.Printf("Ready:       %d\n", len(ready))
			return nil
		},
	}
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run health checks on gig database and config",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := store.Doctor()
			if err != nil {
				return fmt.Errorf("doctor: %w", err)
			}

			if jsonOutput {
				return printJSON(report)
			}

			fmt.Println("Checking gig health...")

			configPath := gig.DefaultConfigPath()
			if _, err := os.Stat(configPath); err != nil {
				fmt.Printf("  [!] Config not found: %s\n", configPath)
			} else {
				fmt.Printf("  [ok] Config: %s\n", configPath)
			}

			if _, err := os.Stat(cfg.DBPath); err != nil {
				fmt.Printf("  [!] Database not found: %s\n", cfg.DBPath)
			} else {
				fmt.Printf("  [ok] Database: %s\n", cfg.DBPath)
			}

			for _, d := range report.Diagnostics {
				switch d.Level {
				case gig.DiagOK:
					fmt.Printf("  [ok] %s\n", d.Message)
				case gig.DiagWarn:
					fmt.Printf("  [!]  %s\n", d.Message)
				case gig.DiagFail:
					fmt.Printf("  [!!] %s\n", d.Message)
				}
			}

			if report.HasIssues() {
				fmt.Println("\nSome issues found. Review warnings above.")
			} else {
				fmt.Println("\nAll checks passed.")
			}

			return nil
		},
	}
}
