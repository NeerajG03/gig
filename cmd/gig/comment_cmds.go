package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func commentCmd() *cobra.Command {
	var author string

	cmd := &cobra.Command{
		Use:               "comment <id> <message>",
		Short:             "Add a comment to a task",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := store.AddComment(args[0], author, args[1])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(c)
			}
			fmt.Printf("Comment added to %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&author, "author", "", "Comment author")
	return cmd
}

func commentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "comments <id>",
		Short:             "List comments on a task",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: taskIDCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			comments, err := store.ListComments(args[0])
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(comments)
			}
			if len(comments) == 0 {
				fmt.Println("No comments.")
				return nil
			}
			for _, c := range comments {
				author := c.Author
				if author == "" {
					author = "anonymous"
				}
				fmt.Printf("[%s] %s: %s\n", c.CreatedAt.Format(timeFormatFull), author, c.Content)
			}
			return nil
		},
	}
}
