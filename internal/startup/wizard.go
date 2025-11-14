package startup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fbettag/pfui/internal/authflow"
	"github.com/fbettag/pfui/internal/authstore"
	"github.com/fbettag/pfui/internal/config"
)

// Run presents the configuration wizard (uses an alternate screen).
func Run(ctx context.Context, cfg config.Config, cfgPath string) error {
	model := newWizardModel(ctx, cfg, cfgPath)
	if _, err := tea.NewProgram(model, tea.WithContext(ctx), tea.WithAltScreen()).Run(); err != nil {
		return err
	}
	if err := ensureConfigFile(cfgPath); err != nil {
		return err
	}
	fmt.Println("Configuration complete. Restart pfui without --configuration to begin chatting.")
	return nil
}

func ensureConfigFile(path string) error {
	var err error
	if strings.TrimSpace(path) == "" {
		path, err = config.DefaultPath()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		fmt.Printf("Configuration preserved at %s\n", path)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	fmt.Printf("Saving example config to %s ...\n", path)
	return config.SaveExample(path)
}

type wizardMode int

const (
	modeList wizardMode = iota
	modeInput
)

type wizardCard struct {
	Title       string
	Description string
	Kind        cardKind
}

type cardKind int

const (
	cardClaudeSubscription cardKind = iota
	cardClaudeAPIKey
	cardOpenAISubscription
	cardOpenAIAPIKey
	cardCustomProvider
	cardMCP
	cardPlanSettings
)

type wizardModel struct {
	ctx              context.Context
	cards            []wizardCard
	selected         int
	message          string
	mode             wizardMode
	input            textinput.Model
	current          *wizardCard
	pendingAnthropic *authflow.AnthropicAuthorize
	authStatus       map[cardKind]bool
	cfg              config.Config
	cfgPath          string
}

func newWizardModel(ctx context.Context, cfg config.Config, cfgPath string) wizardModel {
	ti := textinput.New()
	ti.Prompt = "› "
	ti.Placeholder = "sk-..."
	ti.CharLimit = 256
	status := detectAuthStatus()
	return wizardModel{
		ctx: ctx,
		cards: []wizardCard{
			{"Claude Subscription", "Sign in with your Claude plan (Plus, Pro, Team, Enterprise).", cardClaudeSubscription},
			{"Claude API Key", "Paste Anthropic keys for usage-based workflows.", cardClaudeAPIKey},
			{"OpenAI Subscription", "OAuth into ChatGPT (Plus/Pro/Team) for GPT-5-Codex access.", cardOpenAISubscription},
			{"OpenAI API Key", "Use pure API key auth for automation/CI.", cardOpenAIAPIKey},
			{"Custom Provider", "Bridge z.ai or other connectors via adapter manifests.", cardCustomProvider},
			{"MCP Servers", "Attach user/project scoped MCP servers for plugins.", cardMCP},
			{"Plan Storage", "Decide whether pfui mirrors /plan steps into PLAN.md.", cardPlanSettings},
		},
		message:    "Use ↑/↓ to select. Press enter to configure, esc to exit.",
		input:      ti,
		authStatus: status,
		cfg:        cfg,
		cfgPath:    cfgPath,
	}
}

func detectAuthStatus() map[cardKind]bool {
	status := map[cardKind]bool{}
	creds, err := authstore.Snapshot()
	if err != nil {
		return status
	}
	if tokens, ok := creds.OAuth["openai"]; ok && (tokens.AccessToken != "" || tokens.RefreshToken != "") {
		status[cardOpenAISubscription] = true
	}
	if key := creds.APIKeys["openai"]; key != "" && !status[cardOpenAISubscription] {
		status[cardOpenAIAPIKey] = true
	}
	if tokens, ok := creds.OAuth["anthropic"]; ok && (tokens.AccessToken != "" || tokens.RefreshToken != "") {
		status[cardClaudeSubscription] = true
		if tokens.Extra != nil && tokens.Extra["has_1m_context"] == "true" {
			status[cardClaudeSubscription] = true
		}
	}
	if key := creds.APIKeys["anthropic"]; key != "" && !status[cardClaudeSubscription] {
		status[cardClaudeAPIKey] = true
	}
	return status
}

func (m wizardModel) Init() tea.Cmd {
	return textinput.Blink
}

type openaiAuthMsg struct {
	err  error
	note string
}

type anthropicAuthMsg struct {
	result authflow.AnthropicResult
	err    error
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)
	case openaiAuthMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("OpenAI auth error: %v", msg.err)
		} else {
			m.message = "Linked ChatGPT subscription and stored a fresh OpenAI API key."
			if msg.note != "" {
				m.message += "\n" + msg.note
			}
			m.markConfigured(cardOpenAISubscription)
		}
	case anthropicAuthMsg:
		m.mode = modeList
		m.input.Reset()
		m.current = nil
		m.pendingAnthropic = nil
		if msg.err != nil {
			m.message = fmt.Sprintf("Anthropic auth failed: %v", msg.err)
			return m, nil
		}
		if msg.result.Type == "api" {
			m.message = "Generated and stored a new Claude API key."
			m.markConfigured(cardClaudeAPIKey)
		} else {
			if msg.result.HasMillionCtx {
				m.message = "Linked Claude Pro/Max subscription (1M context)."
			} else {
				m.message = "Stored Claude OAuth tokens."
			}
			m.markConfigured(cardClaudeSubscription)
		}
	}
	return m, nil
}

func (m wizardModel) handleKey(msg tea.KeyMsg) (wizardModel, tea.Cmd) {
	if m.mode == modeInput {
		switch msg.String() {
		case "esc":
			m.mode = modeList
			m.input.Reset()
			m.current = nil
			m.pendingAnthropic = nil
			m.message = "Canceled input."
			return m, nil
		case "enter":
			return m.saveInput()
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "ctrl+c", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.cards)-1 {
			m.selected++
		}
	case "enter":
		return m.activateCard()
	}
	return m, nil
}

func (m wizardModel) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	cardStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(1, 2).
		Width(60)
	activeStyle := cardStyle.Copy().BorderForeground(lipgloss.Color("212")).Bold(true)

	var b strings.Builder
	b.WriteString(titleStyle.Render("pfui configuration wizard"))
	b.WriteByte('\n')
	b.WriteString("This full-screen mode may clear scrollback. Configure providers, API keys, custom adapters, and MCP servers here.\n\n")

	for i, card := range m.cards {
		style := cardStyle
		if i == m.selected {
			style = activeStyle
		}
		prefix := ""
		if m.authStatus[card.Kind] {
			prefix = "✓ "
		}
		desc := card.Description
		if card.Kind == cardPlanSettings {
			desc = fmt.Sprintf("Current: %s", m.planSummary())
		}
		content := fmt.Sprintf("%s%s\n%s", prefix, card.Title, desc)
		b.WriteString(style.Render(content))
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	if m.mode == modeInput {
		b.WriteString(fmt.Sprintf("%s\n", m.input.View()))
		b.WriteString("[enter] save  [esc] cancel\n")
	} else {
		b.WriteString(m.message)
		b.WriteByte('\n')
		b.WriteString("[enter] configure  [esc] exit  [↑/↓] navigate\n")
	}
	return b.String()
}

func (m wizardModel) activateCard() (wizardModel, tea.Cmd) {
	card := m.cards[m.selected]
	switch card.Kind {
	case cardOpenAISubscription:
		return m.startOpenAISubscription()
	case cardClaudeSubscription:
		return m.startAnthropicSubscription(&m.cards[m.selected])
	case cardOpenAIAPIKey:
		return m.startAPIKeyEntry(&card, "Enter OpenAI API key (sk-...)", "openai")
	case cardClaudeAPIKey:
		return m.startAPIKeyEntry(&card, "Enter Claude API key", "anthropic")
	case cardCustomProvider:
		m.message = "Use `pfui provider init` to register adapters today. GUI form coming soon."
	case cardMCP:
		m.message = "Use `pfui mcp add` to manage MCP servers until the form is ready."
	case cardPlanSettings:
		return m.startPlanSettings(&m.cards[m.selected])
	}
	return m, nil
}

func (m wizardModel) startOpenAISubscription() (wizardModel, tea.Cmd) {
	session, err := authflow.StartOpenAICodexFlow(m.ctx)
	if err != nil {
		m.message = fmt.Sprintf("OpenAI auth init error: %v", err)
		return m, nil
	}
	m.message = fmt.Sprintf("Opening OpenAI login. If your browser does not open automatically, visit:\n%s\nIf you're on a remote host, forward port 1455 first: ssh -L 1455:localhost:1455 user@server", session.URL)
	return m, func() tea.Msg {
		_ = authflow.AttemptBrowserOpen(session.URL)
		note, err := session.Wait()
		return openaiAuthMsg{err: err, note: note}
	}
}

func (m wizardModel) startAnthropicSubscription(card *wizardCard) (wizardModel, tea.Cmd) {
	auth, err := authflow.PrepareAnthropicFlow(authflow.AnthropicModeMax)
	if err != nil {
		m.message = fmt.Sprintf("Anthropic auth init error: %v", err)
		return m, nil
	}
	m.mode = modeInput
	m.current = card
	m.pendingAnthropic = auth
	m.input.Placeholder = "Paste the code#state shown in the browser"
	m.input.SetValue("")
	m.input.Focus()
	m.message = fmt.Sprintf("Follow the Claude auth prompts, then paste the code. If your browser stays closed, open:\n%s", auth.URL)
	_ = authflow.AttemptBrowserOpen(auth.URL)
	return m, textinput.Blink
}

func (m wizardModel) startAPIKeyEntry(card *wizardCard, placeholder string, provider string) (wizardModel, tea.Cmd) {
	m.mode = modeInput
	m.current = card
	m.input.Placeholder = placeholder
	m.input.SetValue("")
	m.input.Focus()
	m.message = fmt.Sprintf("Type %s and press Enter", card.Title)
	m.input.CharLimit = 256
	return m, textinput.Blink
}

func (m wizardModel) startPlanSettings(card *wizardCard) (wizardModel, tea.Cmd) {
	m.mode = modeInput
	m.current = card
	m.input.Placeholder = "memory | file [PLAN.md]"
	m.input.SetValue("")
	m.input.Focus()
	m.message = fmt.Sprintf("Current plan storage: %s. Enter 'memory' to keep plans in pfui or 'file [path]' to mirror PLAN.md.", m.planSummary())
	return m, textinput.Blink
}

func (m wizardModel) saveInput() (wizardModel, tea.Cmd) {
	value := strings.TrimSpace(m.input.Value())
	if value == "" {
		m.message = "Input cannot be empty"
		return m, nil
	}
	if m.current == nil {
		m.mode = modeList
		return m, nil
	}
	if m.current.Kind == cardClaudeSubscription {
		if m.pendingAnthropic == nil {
			m.message = "Anthropic auth session expired. Re-open the card to restart."
			return m, nil
		}
		m.message = "Exchanging Claude authorization code..."
		return m, m.exchangeAnthropicCode(value)
	}
	if m.current.Kind == cardPlanSettings {
		if err := m.applyPlanSetting(value); err != nil {
			m.message = fmt.Sprintf("Plan setting error: %v", err)
		} else {
			m.message = fmt.Sprintf("Plan storage updated (%s)", m.planSummary())
		}
		m.mode = modeList
		m.input.Reset()
		m.current = nil
		return m, nil
	}
	var provider string
	switch m.current.Kind {
	case cardOpenAIAPIKey:
		provider = "openai"
	case cardClaudeAPIKey:
		provider = "anthropic"
	default:
		provider = "custom"
	}
	if err := authstore.SaveAPIKey(provider, value); err != nil {
		m.message = fmt.Sprintf("error saving key: %v", err)
		return m, nil
	}
	switch provider {
	case "openai":
		m.markConfigured(cardOpenAIAPIKey)
	case "anthropic":
		m.markConfigured(cardClaudeAPIKey)
	}
	m.mode = modeList
	m.input.Reset()
	m.current = nil
	m.message = fmt.Sprintf("Stored API key for %s", provider)
	return m, nil
}

func (m wizardModel) exchangeAnthropicCode(code string) tea.Cmd {
	auth := m.pendingAnthropic
	return func() tea.Msg {
		result, err := authflow.CompleteAnthropicFlow(auth, code)
		return anthropicAuthMsg{result: result, err: err}
	}
}

func (m *wizardModel) markConfigured(kind cardKind) {
	m.authStatus[kind] = true
}

func (m wizardModel) planSummary() string {
	mode := strings.ToLower(strings.TrimSpace(m.cfg.Plan.Storage))
	switch mode {
	case "file":
		path := m.cfg.Plan.FilePath
		if strings.TrimSpace(path) == "" {
			path = "PLAN.md"
		}
		policy := "manual save"
		if m.cfg.Plan.AutoWrite {
			policy = "auto-save"
		}
		return fmt.Sprintf("file → %s (%s)", path, policy)
	default:
		return "memory only"
	}
}

func (m *wizardModel) applyPlanSetting(value string) error {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		return fmt.Errorf("enter 'memory' or 'file [path]'")
	}
	mode := strings.ToLower(parts[0])
	switch mode {
	case "memory":
		m.cfg.Plan.Storage = "memory"
		m.cfg.Plan.AutoWrite = false
	case "file", "both":
		m.cfg.Plan.Storage = "file"
		if len(parts) > 1 {
			m.cfg.Plan.FilePath = strings.Join(parts[1:], " ")
		}
		if strings.TrimSpace(m.cfg.Plan.FilePath) == "" {
			m.cfg.Plan.FilePath = "PLAN.md"
		}
		if mode == "both" {
			m.cfg.Plan.AutoWrite = true
		} else if !m.cfg.Plan.AutoWrite {
			m.cfg.Plan.AutoWrite = true
		}
	default:
		return fmt.Errorf("unknown option %s", mode)
	}
	if err := config.Save(m.cfgPath, m.cfg); err != nil {
		return err
	}
	return nil
}
