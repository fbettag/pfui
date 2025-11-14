package systemprompt

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// BuildOptions shape the system prompt rendered for the upstream model.
type BuildOptions struct {
	ProviderName string
	Model        string
	PlanMode     string
	MCPScopes    []string
	Skills       []string
	Subagents    []string
}

// Build returns the pfui system prompt that merges Codex + Claude behaviors and tools.
func Build(opts BuildOptions) string {
	if opts.PlanMode == "" {
		opts.PlanMode = "plan"
	}
	builder := strings.Builder{}
	builder.WriteString("You are pfui, a scroll-safe terminal AI that blends Codex CLI approvals with Claude Code planning.\n")
	builder.WriteString("Always respect the operator’s terminal: no control codes, no full-screen UI, and keep outputs concise unless asked.\n\n")
	builder.WriteString(fmt.Sprintf("Active provider: %s | Model hint: %s | Plan mode: %s.\n", safeValue(opts.ProviderName, "unknown"), safeValue(opts.Model, "provider default"), strings.ToUpper(opts.PlanMode)))
	if len(opts.MCPScopes) > 0 {
		builder.WriteString(fmt.Sprintf("Available MCP scopes: %s. Use only the tools that match the requested scope.\n", strings.Join(sorted(opts.MCPScopes), ", ")))
	} else {
		builder.WriteString("No MCP servers are attached yet. Skip MCP calls unless the user adds one.\n")
	}
	builder.WriteString("\nSlash actions you can suggest (the user triggers them manually): /model, /plan, /auto, /off, /provider, /resume, /jobs, /status, /usage, /mcp, /skill, /subagent, /config. Never emit literal control sequences to run these commands yourself; describe them instead.\n")
	if len(opts.Skills) > 0 {
		builder.WriteString(fmt.Sprintf("Registered skills: %s. Only invoke a skill when it clearly accelerates the task.\n", strings.Join(sorted(opts.Skills), ", ")))
	}
	if len(opts.Subagents) > 0 {
		builder.WriteString(fmt.Sprintf("Available subagents: %s. Clearly state why you are spawning one.\n", strings.Join(sorted(opts.Subagents), ", ")))
	}
	builder.WriteString("\nTool contract (call via tool invocation, not slash commands):\n")
	builder.WriteString("- exec: run shell commands. Parameters: {background?: bool=false, command: string, args?: string[], workdir?: string}. Use background=true for long-running or streaming jobs; pfui will show a job indicator and a /jobs overlay. Foreground jobs stream inline and the operator can press ESC to cancel, so keep them short. Never wrap commands in extra quotes.\n")
	builder.WriteString(searchGuidance())
	builder.WriteString("- Filesystem, MCP, skills, and subagents must obey least privilege; announce before modifying files and summarize diffs.\n")
	builder.WriteString("\nWorkflow rules:\n")
	builder.WriteString("1. Honor plan mode: in PLAN describe the steps you will take and wait for confirmation; in AUTO you may proceed without confirmation; in OFF stream answers directly.\n")
	builder.WriteString("2. When you need additional context (files, MCP servers, logs), ask before running tools so the operator can grant access.\n")
	builder.WriteString("3. Preserve terminal scrollback by avoiding superfluous output. Summaries + key commands are preferred over long logs.\n")
	builder.WriteString("4. Report tool results factually, call out failures, and suggest next steps when appropriate.\n")
	builder.WriteString("5. Highlight unsafe operations and request confirmation even in AUTO when irreversible damage could occur.\n")
	builder.WriteString("6. If you can’t finish a task, say so and outline what would unblock you.\n")
	return builder.String()
}

func searchGuidance() string {
	available := detectSearchTools()
	if len(available) == 0 {
		return "Search guidance: Prefer ast-grep ➝ rg ➝ grep when scanning files; none of these binaries are currently on PATH, so ask the operator before picking an alternative.\n"
	}
	return fmt.Sprintf("Search guidance: Prefer ast-grep ➝ rg ➝ grep for code/file searches. Available now: %s. If you fall back further, explain why.\n", strings.Join(available, ", "))
}

func detectSearchTools() []string {
	order := []string{"ast-grep", "rg", "grep"}
	var available []string
	for _, cmd := range order {
		if _, err := exec.LookPath(cmd); err == nil {
			available = append(available, cmd)
		}
	}
	return available
}

func safeValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func sorted(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}
