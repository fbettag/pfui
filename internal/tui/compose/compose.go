package compose

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultPrompt   = "›"
	maxInputHeight  = 6
	minContentWidth = 10
)

var (
	inputBg  = lipgloss.Color("#2C313D")
	inputFg  = lipgloss.Color("#ECEFF4")
	promptFg = lipgloss.Color("#C1C6D6")
	footerBg = lipgloss.Color("#232836")
	footerFg = lipgloss.Color("#A7ACBC")
)

// Model renders the Codex-style compose box.
type Model struct {
	textarea    textarea.Model
	width       int
	prompt      string
	statusLeft  string
	statusRight string
	infoLine    string
	inputStyle  lipgloss.Style
	footerStyle lipgloss.Style
	infoStyle   lipgloss.Style
	promptStyle lipgloss.Style
}

// New returns an initialized compose model.
func New() Model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.Placeholder = "Describe what you need..."
	ta.CharLimit = 0
	ta.MaxHeight = maxInputHeight
	ta.SetWidth(60)
	ta.SetHeight(1)
	ta.Focus()

	m := Model{
		textarea:    ta,
		prompt:      defaultPrompt,
		statusLeft:  "esc to cancel · ctrl+r history",
		statusRight: "? for shortcuts",
		inputStyle:  lipgloss.NewStyle().Background(inputBg).Foreground(inputFg).Padding(0, 1),
		footerStyle: lipgloss.NewStyle().Background(footerBg).Foreground(footerFg).Padding(0, 1),
		infoStyle:   lipgloss.NewStyle().Foreground(footerFg).Padding(0, 1),
		promptStyle: lipgloss.NewStyle().Foreground(promptFg),
	}
	return m
}

// Update processes Bubble Tea messages.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.enforceHeight()
	return m, cmd
}

func (m *Model) enforceHeight() {
	lines := m.textarea.LineCount()
	if lines < 1 {
		lines = 1
	}
	if lines > maxInputHeight {
		lines = maxInputHeight
	}
	m.textarea.SetHeight(lines)
}

// View renders the compose area including footer/info lines.
func (m Model) View() string {
	body := m.renderBody()
	footer := m.renderFooter()
	parts := []string{body, footer}
	if strings.TrimSpace(m.infoLine) != "" {
		parts = append(parts, m.renderInfo())
	}
	return strings.Join(parts, "\n") + "\n"
}

func (m Model) renderBody() string {
	content := strings.TrimSuffix(m.textarea.View(), "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	styledPrompt := m.promptStyle.Render(m.prompt)
	promptWidth := lipgloss.Width(styledPrompt) + 1
	prompt := styledPrompt + " "
	spacer := strings.Repeat(" ", promptWidth)
	for i, line := range lines {
		trimmed := line
		if i == 0 {
			lines[i] = prompt + trimmed
		} else {
			lines[i] = spacer + trimmed
		}
		lines[i] = m.inputStyle.Width(m.width).Render(lines[i])
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderFooter() string {
	left := strings.TrimSpace(m.statusLeft)
	right := strings.TrimSpace(m.statusRight)
	if left == "" && right == "" {
		return m.footerStyle.Width(m.width).Render("")
	}
	line := left
	if right != "" {
		leftWidth := lipgloss.Width(left)
		rightWidth := lipgloss.Width(right)
		gap := m.width - leftWidth - rightWidth - 2
		if gap < 1 {
			gap = 1
		}
		line = left + strings.Repeat(" ", gap) + right
	}
	return m.footerStyle.Width(m.width).Render(line)
}

func (m Model) renderInfo() string {
	return m.infoStyle.Width(m.width).Render(m.infoLine)
}

// Height returns the total number of lines occupied by the composer (body + footer + optional info).
func (m Model) Height() int {
	infoLines := 0
	if strings.TrimSpace(m.infoLine) != "" {
		infoLines = 1
	}
	return m.textarea.Height() + 1 + infoLines
}

// SetWidth updates the compose width and inner textarea width.
func (m *Model) SetWidth(width int) {
	if width <= 0 {
		width = 40
	}
	m.width = width
	inner := width - 6
	if inner < minContentWidth {
		inner = width - 2
	}
	if inner < minContentWidth {
		inner = minContentWidth
	}
	m.textarea.SetWidth(inner)
}

// Value returns the textarea value.
func (m Model) Value() string {
	return m.textarea.Value()
}

// SetValue replaces the textarea contents.
func (m *Model) SetValue(val string) {
	m.textarea.SetValue(val)
	m.enforceHeight()
}

// Reset clears the textarea.
func (m *Model) Reset() {
	m.textarea.SetValue("")
	m.enforceHeight()
}

// Focus applies focus to the textarea.
func (m *Model) Focus() {
	m.textarea.Focus()
}

// Blur removes focus from the textarea.
func (m *Model) Blur() {
	m.textarea.Blur()
}

// CursorEnd moves the cursor to the end of the textarea.
func (m *Model) CursorEnd() {
	m.textarea.CursorEnd()
}

// SetPlaceholder sets placeholder text.
func (m *Model) SetPlaceholder(text string) {
	m.textarea.Placeholder = text
}

// SetPrompt updates the prompt glyph.
func (m *Model) SetPrompt(p string) {
	if strings.TrimSpace(p) == "" {
		p = defaultPrompt
	}
	m.prompt = p
}

// SetStatus configures the footer status text.
func (m *Model) SetStatus(left, right string) {
	m.statusLeft = left
	m.statusRight = right
}

// SetInfoLine configures the metadata line rendered below the footer.
func (m *Model) SetInfoLine(text string) {
	m.infoLine = strings.TrimSpace(text)
}
