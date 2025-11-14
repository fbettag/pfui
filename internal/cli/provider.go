package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fbettag/pfui/internal/provider"
)

func newProviderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage custom providers",
	}
	cmd.AddCommand(newProviderInitCommand())
	return cmd
}

func newProviderInitCommand() *cobra.Command {
	var adapter string
	var host string
	var token string
	cmd := &cobra.Command{
		Use:   "init NAME",
		Short: "Create a provider manifest skeleton",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path, err := provider.InitProvider(provider.Manifest{
				Name:    name,
				Adapter: provider.AdapterKind(adapter),
				Host:    host,
				Token:   token,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&adapter, "adapter", string(provider.AdapterOpenAIChat), "Adapter kind (openai-chat|openai-responses|anthropic-messages)")
	cmd.Flags().StringVar(&host, "host", "", "Provider hostname/base URL")
	cmd.Flags().StringVar(&token, "token", "", "Bearer/API token (stored locally)")
	return cmd
}
