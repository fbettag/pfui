package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/fbettag/pfui/internal/config"
	execpkg "github.com/fbettag/pfui/internal/exec"
)

func newExecCommand() *cobra.Command {
	var cfgFileOverride string
	var auto bool

	cmd := &cobra.Command{
		Use:   "exec [prompt]",
		Short: "Run pfui in non-interactive mode",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runExec(ctx, cfgFileOverride, args[0], auto)
		},
	}
	cmd.Flags().StringVar(&cfgFileOverride, "config", "", "Path to pfui config file")
	cmd.Flags().BoolVar(&auto, "auto", false, "Run without confirmations")
	return cmd
}

func runExec(ctx context.Context, cfgPath string, prompt string, auto bool) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	return execpkg.Run(ctx, execpkg.Options{
		Config:      cfg,
		Prompt:      prompt,
		AutoApprove: auto,
	})
}
