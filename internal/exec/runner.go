package exec

import (
	"context"
	"fmt"

	"github.com/fbettag/pfui/internal/config"
)

// Options configure exec mode.
type Options struct {
	Config      config.Config
	Prompt      string
	AutoApprove bool
}

// Run currently streams a placeholder response to demonstrate wiring between the CLI and backend.
func Run(ctx context.Context, opts Options) error {
	if opts.Prompt == "" {
		return fmt.Errorf("prompt is required")
	}
	auto := "off"
	if opts.AutoApprove {
		auto = "on"
	}
	fmt.Printf("[pfui exec] prompt=%q auto=%s whitelist=%d models\n", opts.Prompt, auto, len(opts.Config.Models.Whitelist))
	// TODO: integrate provider/session execution.
	return nil
}
