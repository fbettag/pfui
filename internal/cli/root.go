package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/fbettag/pfui/internal/config"
	"github.com/fbettag/pfui/internal/history"
	"github.com/fbettag/pfui/internal/providersetup"
	"github.com/fbettag/pfui/internal/startup"
	"github.com/fbettag/pfui/internal/tui"
)

const resumePickerSentinel = "__pfui_resume_picker__"

var (
	cfgFile       string
	runConfigMode bool
	resumeID      string
)

// Execute boots the CLI.
func Execute() {
	root := newRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "pfui: %v\n", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "pfui",
		Short:        "pfui is a provider-agnostic coding TUI",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			return runRoot(ctx)
		},
	}
	cmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to pfui config file (defaults to ~/.pfui/config.toml)")
	cmd.Flags().BoolVar(&runConfigMode, "configuration", false, "Launch configuration wizard (clears scrollback)")
	cmd.Flags().StringVar(&resumeID, "resume", "", "Resume a previous chat by UUID (omit to pick from history)")
	cmd.Flags().Lookup("resume").NoOptDefVal = resumePickerSentinel

	cmd.AddCommand(
		newExecCommand(),
		newProviderCommand(),
		newMCPCommand(),
		newAuthCommand(),
	)

	return cmd
}

func runRoot(ctx context.Context) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return err
	}
	configPath := cfgFile
	if strings.TrimSpace(configPath) == "" {
		configPath, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	projectPath, err := os.Getwd()
	if err != nil {
		return err
	}
	if runConfigMode {
		return startup.Run(ctx, cfg, configPath)
	}
	launchArgs := sanitizeLaunchArgs(os.Args[1:])
	providers := providersetup.DefaultRegistry(cfg)
	if resumeID == resumePickerSentinel {
		sessions, err := history.List(projectPath)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			return fmt.Errorf("no history available for %s", projectPath)
		}
		selected, err := history.Select(ctx, sessions, history.PickerConfig{
			Title: fmt.Sprintf("Resume a chat in %s", projectPath),
		})
		if err != nil {
			return err
		}
		resumeID = selected.ID
	}
	return tui.Run(ctx, cfg, tui.Options{
		ResumeID:    resumeID,
		ProjectPath: projectPath,
		Providers:   providers,
		LaunchArgs:  launchArgs,
	})
}

func sanitizeLaunchArgs(args []string) string {
	var filtered []string
	skip := false
	for _, arg := range args {
		if skip {
			skip = false
			continue
		}
		if strings.HasPrefix(arg, "--resume") {
			if arg == "--resume" {
				skip = true
			}
			continue
		}
		filtered = append(filtered, arg)
	}
	return strings.Join(filtered, " ")
}
