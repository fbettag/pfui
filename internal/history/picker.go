package history

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PickerConfig configures the resume picker UI.
type PickerConfig struct {
	Title string
}

// Select launches a picker for the provided sessions, returning the chosen entry.
func Select(ctx context.Context, sessions []Session, cfg PickerConfig) (Session, error) {
	if len(sessions) == 0 {
		return Session{}, fmt.Errorf("no sessions available")
	}
	model := pickerModel{
		title:     cfg.Title,
		sessions:  sessions,
		filtered:  sessions,
		searchBox: newSearchInput(),
	}
	p := tea.NewProgram(model, tea.WithContext(ctx))
	finalModel, err := p.Run()
	if err != nil {
		return Session{}, err
	}
	pm, ok := finalModel.(pickerModel)
	if !ok || pm.selected == nil {
		return Session{}, fmt.Errorf("no session selected")
	}
	return *pm.selected, nil
}

type pickerModel struct {
	title     string
	sessions  []Session
	filtered  []Session
	cursor    int
	searching bool
	searchBox textinput.Model
	selected  *Session
}

func newSearchInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Search (/ or Ctrl+R)"
	ti.Prompt = "/ "
	ti.CharLimit = 256
	return ti
}

func (m pickerModel) Init() tea.Cmd {
	return nil
}

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			if m.searching {
				m.searching = false
				m.searchBox.Reset()
				m.filtered = m.sessions
				m.cursor = 0
				return m, nil
			}
			return m, tea.Quit
		case "j", "down":
			m.moveCursor(1)
			return m, nil
		case "k", "up":
			m.moveCursor(-1)
			return m, nil
		case "enter":
			if len(m.filtered) == 0 {
				return m, nil
			}
			selected := m.filtered[m.cursor]
			m.selected = &selected
			return m, tea.Quit
		case "/", "ctrl+r":
			m.searching = true
			m.searchBox.Focus()
			return m, nil
		default:
			if m.searching {
				var cmd tea.Cmd
				old := m.searchBox.Value()
				m.searchBox, cmd = m.searchBox.Update(msg)
				if old != m.searchBox.Value() {
					m.applyFilter()
				}
				return m, cmd
			}
		}
	case tea.WindowSizeMsg:
		return m, nil
	}
	return m, nil
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.title == "" {
		b.WriteString("Select a session (arrows, / search, enter to resume, esc to cancel)\n")
	} else {
		b.WriteString(fmt.Sprintf("%s\n", m.title))
	}
	if m.searching {
		b.WriteString(m.searchBox.View())
		b.WriteByte('\n')
	}
	for i, session := range m.filtered {
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		b.WriteString(fmt.Sprintf("%s%s [%s]\n", prefix, session.Title, session.ID))
	}
	return b.String()
}

func (m *pickerModel) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
}

func (m *pickerModel) applyFilter() {
	query := strings.ToLower(strings.TrimSpace(m.searchBox.Value()))
	if query == "" {
		m.filtered = m.sessions
		m.cursor = 0
		return
	}
	var filtered []Session
	for _, session := range m.sessions {
		if strings.Contains(strings.ToLower(session.Title), query) ||
			strings.Contains(strings.ToLower(session.Summary), query) ||
			strings.Contains(strings.ToLower(session.ID), query) {
			filtered = append(filtered, session)
		}
	}
	m.filtered = filtered
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
