package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/fbettag/pfui/internal/authflow"
	"github.com/fbettag/pfui/internal/authstore"
)

func newAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Inspect or manage authentication state",
	}
	cmd.AddCommand(newAuthStatusCommand(), newAuthRefreshCommand())
	return cmd
}

func newAuthStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cached auth providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			creds, err := authstore.Snapshot()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Stored credentials:")
			printProviderStatus(out, "OpenAI", creds)
			printProviderStatus(out, "Anthropic", creds)
			return nil
		},
	}
}

func newAuthRefreshCommand() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh OAuth tokens and API keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthRefresh(cmd.Context(), strings.ToLower(provider), cmd)
		},
	}
	cmd.Flags().StringVar(&provider, "provider", "all", "Provider to refresh (openai|anthropic|all)")
	return cmd
}

func runAuthRefresh(ctx context.Context, provider string, cmd *cobra.Command) error {
	creds, err := authstore.Snapshot()
	if err != nil {
		return err
	}
	providers := []string{"openai", "anthropic"}
	if provider != "" && provider != "all" {
		providers = []string{provider}
	}
	for _, p := range providers {
		switch p {
		case "openai":
			tokens, ok := creds.OAuth[p]
			if !ok {
				fmt.Fprintf(cmd.OutOrStdout(), "OpenAI: no OAuth tokens stored. Run `pfui --configuration` first.\n")
				continue
			}
			newTokens, apiKey, err := authflow.RefreshOpenAITokens(tokens)
			if err != nil {
				return fmt.Errorf("refresh OpenAI tokens: %w", err)
			}
			if err := authstore.SaveOAuthTokens("openai", newTokens); err != nil {
				return err
			}
			if apiKey != "" {
				if err := authstore.SaveAPIKey("openai", apiKey); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "OpenAI: refreshed tokens and minted new API key (%s).\n", maskKey(apiKey))
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "OpenAI: refreshed tokens.\n")
			}
		case "anthropic":
			tokens, ok := creds.OAuth[p]
			if !ok {
				fmt.Fprintf(cmd.OutOrStdout(), "Anthropic: no OAuth tokens stored. Run `pfui --configuration`.\n")
				continue
			}
			newTokens, err := authflow.RefreshAnthropicTokens(tokens)
			if err != nil {
				return fmt.Errorf("refresh Anthropic tokens: %w", err)
			}
			if err := authstore.SaveOAuthTokens("anthropic", newTokens); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Anthropic: refreshed tokens (expires %s).\n", humanizeExpiry(newTokens.ExpiresAt))
		default:
			fmt.Fprintf(cmd.OutOrStdout(), "Unknown provider %s.\n", p)
		}
	}
	return nil
}

func printProviderStatus(out io.Writer, provider string, creds authstore.Credentials) {
	switch strings.ToLower(provider) {
	case "openai":
		key, hasKey := creds.APIKeys["openai"]
		tokens, hasTokens := creds.OAuth["openai"]
		fmt.Fprintf(out, "OpenAI: ")
		if hasKey {
			fmt.Fprintf(out, "API key %s", maskKey(key))
		} else {
			fmt.Fprint(out, "no API key")
		}
		if hasTokens {
			fmt.Fprintf(out, ", tokens expire %s", humanizeExpiry(tokens.ExpiresAt))
		}
		fmt.Fprintln(out)
	case "anthropic":
		key, hasKey := creds.APIKeys["anthropic"]
		tokens, hasTokens := creds.OAuth["anthropic"]
		fmt.Fprintf(out, "Anthropic: ")
		if hasKey {
			fmt.Fprintf(out, "API key %s", maskKey(key))
		} else {
			fmt.Fprint(out, "no API key")
		}
		if hasTokens {
			fmt.Fprintf(out, ", tokens expire %s", humanizeExpiry(tokens.ExpiresAt))
			if tokens.Extra != nil && tokens.Extra["has_1m_context"] == "true" {
				fmt.Fprint(out, " (Claude 1M context)")
			}
		}
		fmt.Fprintln(out)
	default:
		fmt.Fprintf(out, "%s: no information\n", provider)
	}
}

func humanizeExpiry(ts int64) string {
	if ts == 0 {
		return "(unknown)"
	}
	d := time.Until(time.Unix(ts, 0))
	if d <= 0 {
		return "(expired)"
	}
	return fmt.Sprintf("in %s", d.Round(time.Minute))
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return key
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}
