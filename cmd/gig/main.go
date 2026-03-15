package main

import (
	"fmt"
	"os"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

var version = "dev"

var (
	jsonOutput  bool
	quietOutput bool
	actorName   string
	store       *gig.Store
	cfg         *gig.Config
)

func main() {
	rootCmd := &cobra.Command{
		Use:     "gig",
		Short:   "A lightweight task management system",
		Long:    "gig — task management CLI & SDK. Tracks tasks, dependencies, and events with SQLite.",
		Version: version,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Skip store init for commands that don't need it.
			if cmd.Name() == "init" || cmd.Name() == "completion" {
				return nil
			}
			var err error
			cfg, err = gig.LoadConfig("")
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			s, err := gig.Open(cfg.DBPath, gig.WithPrefix(cfg.Prefix), gig.WithHashLength(cfg.HashLen), gig.WithConfig(cfg))
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			store = s
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if store != nil {
				store.Close()
			}
		},
	}

	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&quietOutput, "quiet", "q", false, "Output IDs only")
	rootCmd.PersistentFlags().StringVar(&actorName, "actor", "cli", "Actor name for event attribution")

	rootCmd.AddCommand(
		initCmd(),
		createCmd(),
		listCmd(),
		showCmd(),
		updateCmd(),
		closeCmd(),
		reopenCmd(),
		commentCmd(),
		commentsCmd(),
		depCmd(),
		readyCmd(),
		blockedCmd(),
		childrenCmd(),
		exportCmd(),
		importCmd(),
		syncCmd(),
		eventsCmd(),
		statsCmd(),
		configCmd(),
		doctorCmd(),
		uiCmd(),
		attrCmd(),
		searchCmd(),
		completionCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
