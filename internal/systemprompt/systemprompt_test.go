package systemprompt

import (
	"strings"
	"testing"
)

func TestBuildIncludesExecTool(t *testing.T) {
	prompt := Build(BuildOptions{
		ProviderName: "OpenAI",
		Model:        "gpt-5.1-codex",
		PlanMode:     "plan",
		MCPScopes:    []string{"project", "user"},
		Skills:       []string{"unit-tests"},
		Subagents:    []string{"code-search"},
	})
	if !strings.Contains(prompt, "exec: run shell commands") {
		t.Fatalf("prompt missing exec tool description: %s", prompt)
	}
	if !strings.Contains(prompt, "PLAN describe the steps") {
		t.Fatalf("prompt missing plan instructions: %s", prompt)
	}
	if !strings.Contains(prompt, "project, user") {
		t.Fatalf("prompt missing MCP scopes: %s", prompt)
	}
}
