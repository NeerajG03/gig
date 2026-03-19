package main

import (
	"fmt"

	"github.com/NeerajG03/gig/ui"
	"github.com/spf13/cobra"
)

func uiCmd() *cobra.Command {
	var port int

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start the web-based kanban board UI",
		Long:  "Launches a local web server with a drag-and-drop kanban board for managing tasks.",
		RunE: func(cmd *cobra.Command, args []string) error {
			server := ui.New(store)
			addr := fmt.Sprintf(":%d", port)
			fmt.Printf("gig ui running at http://localhost:%d\n", port)
			return server.ListenAndServe(addr)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", 9741, "Port to listen on")

	return cmd
}
