package main

import (
	"github.com/NeerajG03/gig"
	"github.com/spf13/cobra"
)

// completionCmd generates shell completion scripts.
func completionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate shell completion script",
		Long: `Generate shell completion scripts for gig.

To load completions:

  bash:
    source <(gig completion bash)
    # Or add to ~/.bashrc:
    echo 'source <(gig completion bash)' >> ~/.bashrc

  zsh:
    source <(gig completion zsh)
    # Or install permanently:
    gig completion zsh > "${fpath[1]}/_gig"

  fish:
    gig completion fish | source
    # Or install permanently:
    gig completion fish > ~/.config/fish/completions/gig.fish
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish"},
		Args:                  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return cmd.Help()
			}
		},
	}
	return cmd
}

// taskIDCompletion returns a ValidArgsFunction that completes task IDs.
func taskIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if store == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	tasks, err := store.List(gig.ListParams{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var ids []string
	for _, t := range tasks {
		ids = append(ids, t.ID+"\t"+t.Title)
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// openTaskIDCompletion completes only non-closed task IDs.
func openTaskIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if store == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	tasks, err := store.List(gig.ListParams{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var ids []string
	for _, t := range tasks {
		if t.Status != gig.StatusClosed {
			ids = append(ids, t.ID+"\t"+t.Title)
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// closedTaskIDCompletion completes closed and cancelled task IDs (for reopen).
func closedTaskIDCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if store == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	closed := gig.StatusClosed
	cancelled := gig.StatusCancelled
	closedTasks, _ := store.List(gig.ListParams{Status: &closed})
	cancelledTasks, _ := store.List(gig.ListParams{Status: &cancelled})
	var ids []string
	for _, t := range append(closedTasks, cancelledTasks...) {
		ids = append(ids, t.ID+"\t"+t.Title)
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}

// attrKeyCompletion completes defined attribute keys.
func attrKeyCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if store == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	defs, err := store.ListAttrDefs()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var keys []string
	for _, d := range defs {
		keys = append(keys, d.Key+"\t"+d.Description)
	}
	return keys, cobra.ShellCompDirectiveNoFileComp
}

// statusCompletion completes valid status values.
func statusCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"open", "in_progress", "blocked", "deferred", "closed", "cancelled"}, cobra.ShellCompDirectiveNoFileComp
}

// taskTypeCompletion completes valid task type values.
func taskTypeCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"task", "bug", "feature", "epic", "chore"}, cobra.ShellCompDirectiveNoFileComp
}

// priorityCompletion completes valid priority values.
func priorityCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"0\tcritical", "1\thigh", "2\tmedium", "3\tlow", "4\tbacklog"}, cobra.ShellCompDirectiveNoFileComp
}

// attrTypeCompletion completes valid attribute types.
func attrTypeCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"string", "boolean", "object"}, cobra.ShellCompDirectiveNoFileComp
}
