package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fbettag/pfui/internal/mcp"
)

func newMCPCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Manage MCP servers",
	}
	cmd.AddCommand(newMCPAddCommand())
	return cmd
}

func newMCPAddCommand() *cobra.Command {
	var scope string
	var url string
	cmd := &cobra.Command{
		Use:   "add NAME",
		Short: "Add an MCP server descriptor",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path, err := mcp.AddServer(mcp.Scope(scope), mcp.Server{
				Name: name,
				URL:  url,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Registered MCP server at %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", string(mcp.ScopeUser), "Scope for registration (user|project)")
	cmd.Flags().StringVar(&url, "url", "", "MCP server URL")
	cmd.MarkFlagRequired("url")
	return cmd
}
