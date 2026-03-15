package main

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/neerajg/gig"
	"github.com/spf13/cobra"
)

func attrCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attr",
		Short: "Manage custom attributes",
		Long:  "Define attribute types and set/get typed key-value pairs on tasks.",
	}

	cmd.AddCommand(
		attrDefineCmd(),
		attrUndefineCmd(),
		attrTypesCmd(),
		attrSetCmd(),
		attrGetCmd(),
		attrListCmd(),
		attrDeleteCmd(),
	)

	return cmd
}

func attrDefineCmd() *cobra.Command {
	var attrType string
	var description string

	cmd := &cobra.Command{
		Use:   "define <key>",
		Short: "Define a new attribute type",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			t := gig.AttrType(attrType)
			if !t.IsValid() {
				return fmt.Errorf("invalid type %q, must be: string, boolean, object", attrType)
			}
			if err := store.DefineAttr(key, t, description); err != nil {
				return err
			}
			if !quietOutput {
				fmt.Printf("Defined attribute %q (type: %s)\n", key, attrType)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&attrType, "type", "string", "Attribute type: string, boolean, object")
	cmd.Flags().StringVar(&description, "description", "", "Description of the attribute")

	return cmd
}

func attrUndefineCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "undefine <key>",
		Short: "Remove an attribute definition and all its values",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.UndefineAttr(args[0]); err != nil {
				return err
			}
			if !quietOutput {
				fmt.Printf("Removed attribute %q and all its values\n", args[0])
			}
			return nil
		},
	}
}

func attrTypesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "types",
		Short: "List all defined attribute types",
		RunE: func(cmd *cobra.Command, args []string) error {
			defs, err := store.ListAttrDefs()
			if err != nil {
				return err
			}

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(defs)
			}

			if len(defs) == 0 {
				fmt.Println("No attribute types defined. Use 'gig attr define <key> --type <type>' to create one.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "KEY\tTYPE\tDESCRIPTION\n")
			for _, d := range defs {
				fmt.Fprintf(w, "%s\t%s\t%s\n", d.Key, d.Type, d.Description)
			}
			return w.Flush()
		},
	}
}

func attrSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <task-id> <key> <value>",
		Short: "Set a custom attribute on a task",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskID, key, value := args[0], args[1], args[2]
			if err := store.SetAttr(taskID, key, value); err != nil {
				return err
			}
			if !quietOutput {
				fmt.Printf("%s.%s = %s\n", taskID, key, value)
			}
			return nil
		},
	}
}

func attrGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <task-id> <key>",
		Short: "Get a custom attribute value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			attr, err := store.GetAttr(args[0], args[1])
			if err != nil {
				return err
			}

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(attr)
			}

			fmt.Printf("%s (%s)\n", attr.Value, attr.Type)
			return nil
		},
	}
}

func attrListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <task-id>",
		Short: "List all custom attributes on a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			attrs, err := store.Attrs(args[0])
			if err != nil {
				return err
			}

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(attrs)
			}

			if len(attrs) == 0 {
				fmt.Println("No attributes set.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "KEY\tVALUE\tTYPE\n")
			for _, a := range attrs {
				fmt.Fprintf(w, "%s\t%s\t%s\n", a.Key, a.Value, a.Type)
			}
			return w.Flush()
		},
	}
}

func attrDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <task-id> <key>",
		Short: "Remove a custom attribute from a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := store.DeleteAttr(args[0], args[1]); err != nil {
				return err
			}
			if !quietOutput {
				fmt.Printf("Deleted %s.%s\n", args[0], args[1])
			}
			return nil
		},
	}
}
