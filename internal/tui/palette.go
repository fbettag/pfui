package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type commandPalette struct {
	visible          bool
	filter           string
	commands         []string
	filteredCommands []string
	SelectedCommand  string
}

var defaultCommands = []string{
	"/model",
	"/plan",
	"/auto",
	"/off",
	"/provider",
	"/resume",
	"/config",
	"/status",
	"/usage",
	"/jobs",
	"/approvals",
	"/compact",
	"/mcp",
	"/plugin",
	"/skill",
	"/subagent",
	"/help",
}

func newCommandPalette() commandPalette {
	cmds := append([]string(nil), defaultCommands...)
	return commandPalette{
		commands:         cmds,
		filteredCommands: append([]string(nil), cmds...),
	}
}

func (p *commandPalette) activate() {
	p.visible = true
	p.filter = ""
	p.filteredCommands = append([]string(nil), p.commands...)
}

func (p *commandPalette) UpdateKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.Type {
	case tea.KeyRunes:
		p.setFilter(p.filter + string(msg.Runes))
		return true, nil
	case tea.KeyBackspace:
		if len(p.filter) > 0 {
			p.setFilter(p.filter[:len(p.filter)-1])
		}
		return true, nil
	case tea.KeyEnter:
		if len(p.filteredCommands) > 0 {
			p.SelectedCommand = p.filteredCommands[0]
		}
		p.visible = false
		return true, nil
	case tea.KeyEsc, tea.KeyCtrlC:
		p.Reset()
		return true, nil
	}
	return false, nil
}

func (p *commandPalette) Reset() {
	p.visible = false
	p.filter = ""
	p.filteredCommands = append([]string(nil), p.commands...)
	p.SelectedCommand = ""
}

func (p *commandPalette) View() string {
	if !p.visible {
		return ""
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Slash commands [%s]\n", p.filter))
	for i, cmd := range p.filteredCommands {
		if i >= 6 {
			builder.WriteString("  …\n")
			break
		}
		builder.WriteString("  " + cmd + "\n")
	}
	return builder.String()
}

func (p *commandPalette) setFilter(filter string) {
	p.filter = filter
	p.filteredCommands = p.filterCommands(filter)
	if strings.TrimSpace(filter) != "" && len(p.filteredCommands) == 1 {
		p.SelectedCommand = p.filteredCommands[0]
		p.visible = false
	}
}

func (p *commandPalette) filterCommands(filter string) []string {
	if strings.TrimSpace(filter) == "" {
		return append([]string(nil), p.commands...)
	}
	var filtered []string
	lower := strings.ToLower(filter)
	for _, cmd := range p.commands {
		if strings.Contains(strings.ToLower(cmd), lower) {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}
