package main

import (
	"fmt"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show or update configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := gig.LoadConfig("")
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(cfg)
			}
			fmt.Printf("Home:         %s\n", gig.DefaultGigHome())
			fmt.Printf("Config:       %s\n", gig.DefaultConfigPath())
			fmt.Printf("Database:     %s\n", cfg.DBPath)
			fmt.Printf("Prefix:       %s\n", cfg.Prefix)
			fmt.Printf("Hash Len:     %d\n", cfg.HashLen)
			dv := cfg.DefaultView
			if dv == "" {
				dv = "list"
			}
			fmt.Printf("Default View: %s\n", dv)
			fmt.Printf("Show All:     %v\n", cfg.ShowAll)
			if cfg.SyncRepo != "" {
				fmt.Printf("Sync Repo:    %s\n", cfg.SyncRepo)
			}
			return nil
		},
	}

	cmd.AddCommand(configSetCmd())
	return cmd
}

func configSetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value in gig.yaml.

Supported keys:
  prefix        ID prefix (string)
  hash_length   ID hash length, 3-8 (integer)
  default_view  Default list view: "list" or "tree"
  show_all      Show closed tasks by default: true or false
  sync_repo     Git sync repo path (string)`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]

			c, err := gig.LoadConfig("")
			if err != nil {
				return err
			}

			switch key {
			case "prefix":
				c.Prefix = value
			case "hash_length":
				var n int
				if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
					return fmt.Errorf("hash_length must be an integer: %w", err)
				}
				if n < 3 || n > 8 {
					return fmt.Errorf("hash_length must be between 3 and 8, got %d", n)
				}
				c.HashLen = n
			case "default_view":
				if value != "list" && value != "tree" {
					return fmt.Errorf("default_view must be 'list' or 'tree', got %q", value)
				}
				c.DefaultView = value
			case "show_all":
				switch value {
				case "true":
					c.ShowAll = true
				case "false":
					c.ShowAll = false
				default:
					return fmt.Errorf("show_all must be 'true' or 'false', got %q", value)
				}
			case "sync_repo":
				c.SyncRepo = value
			default:
				return fmt.Errorf("unknown config key %q (valid: prefix, hash_length, default_view, show_all, sync_repo)", key)
			}

			if err := gig.SaveConfig("", c); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			if !quietOutput {
				fmt.Printf("Set %s = %s\n", key, value)
			}
			return nil
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return []string{
					"prefix\tID prefix",
					"hash_length\tID hash length (3-8)",
					"default_view\tDefault list view (list/tree)",
					"show_all\tShow closed tasks by default (true/false)",
					"sync_repo\tGit sync repo path",
				}, cobra.ShellCompDirectiveNoFileComp
			}
			if len(args) == 1 {
				switch args[0] {
				case "default_view":
					return []string{"list", "tree"}, cobra.ShellCompDirectiveNoFileComp
				case "show_all":
					return []string{"true", "false"}, cobra.ShellCompDirectiveNoFileComp
				}
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	return cmd
}
