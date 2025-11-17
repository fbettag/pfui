package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/fbettag/pfui/internal/config"
	"github.com/fbettag/pfui/internal/history"
	"github.com/fbettag/pfui/internal/provider"
	"github.com/fbettag/pfui/internal/toolexec"
	"github.com/fbettag/pfui/internal/tui/compose"
)

// Options configure the interactive chat run.
type Options struct {
	ResumeID    string
	ProjectPath string
	Providers   provider.Registry
	LaunchArgs  string
}

type planMode string

const (
	planModePlan planMode = "plan"
	planModeAuto planMode = "auto"
	planModeOff  planMode = "off"
)

var (
	planBadgeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2563eb")).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true).
			Padding(0, 1)
	autoBadgeStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#ffffff")).
			Foreground(lipgloss.Color("#000000")).
			Bold(true).
			Padding(0, 1)
	userBlockStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2E323F")).
			Foreground(lipgloss.Color("#E1E6F2"))
	assistantBlockStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E6EDF7"))
)

// Run launches the chat interface in the foreground.
func Run(ctx context.Context, cfg config.Config, opts Options) error {
	m := newModel(ctx, cfg, opts)
	p := tea.NewProgram(m, tea.WithContext(ctx))
	finalModel, err := p.Run()
	if fm, ok := finalModel.(model); ok && fm.session.ID != "" {
		printResumeHint(fm.session.ID, opts.LaunchArgs)
	}
	return err
}

type model struct {
	ctx              context.Context
	cfg              config.Config
	opts             Options
	providers        provider.Registry
	available        []provider.Provider
	activeProvider   provider.Provider
	awaitingProvider bool
	defaultModel     string
	commandPalette   commandPalette
	executor         *toolexec.Executor
	jobs             map[string]toolexec.Job
	messages         []string
	compose          compose.Model
	width            int
	height           int
	session          history.Session
	statusLine       string
	promptHistory    []string
	recallMode       bool
	recallPosition   int
	plan             planMode
	planSteps        []planStep
	showPlan         bool
	question         *questionPrompt
	catalog          modelCatalog
	spinner          spinner.Model
	pendingResponse  *streamingResponse
	responseStream   *responseStreamState
	pendingCancel    context.CancelFunc
}

func newModel(ctx context.Context, cfg config.Config, opts Options) model {
	composer := compose.New()
	composer.SetPlaceholder("Describe what you need...")
	composer.Focus()
	composer.SetWidth(80)

	lines := []string{
		"pfui ready. Configuration mode keeps scrollback safe; run `/config` or `pfui --configuration` for the full-screen wizard.",
	}
	session, status := initSession(opts)
	if session.ID != "" {
		lines = append(lines, fmt.Sprintf("Session %s (%s)", session.ID, session.Project))
	}
	available := opts.Providers.Providers()
	awaiting := false
	var active provider.Provider
	defaultModel := ""
	switch len(available) {
	case 0:
		lines = append(lines, "No providers configured. Run `pfui --configuration` to add OpenAI or Claude accounts.")
	case 1:
		active = available[0]
		defaultModel = defaultModelFor(active)
		lines = append(lines, fmt.Sprintf("Using %s via %s", defaultModelDisplay(defaultModel), active.Name()))
	default:
		awaiting = true
		lines = append(lines, providerPromptText(available))
	}
	if active != nil {
		status = fmt.Sprintf("%s | Provider: %s (%s)", status, active.Name(), defaultModelDisplay(defaultModel))
	}
	header := historyBlockLines("pfui session", buildSessionHeaderLines(session, opts.ProjectPath, cfg.Plan, available, planModePlan))
	lines = append(header, lines...)
	executor := toolexec.NewExecutor()
	spin := spinner.New()
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#C1C6D6"))
	m := model{
		ctx:              ctx,
		cfg:              cfg,
		opts:             opts,
		providers:        opts.Providers,
		available:        available,
		activeProvider:   active,
		awaitingProvider: awaiting,
		defaultModel:     defaultModel,
		commandPalette:   newCommandPalette(),
		executor:         executor,
		jobs:             make(map[string]toolexec.Job),
		messages:         lines,
		compose:          composer,
		session:          session,
		statusLine:       status,
		plan:             planModePlan,
		showPlan:         true,
		catalog: modelCatalog{
			loading: make(map[string]bool),
		},
		spinner: spin,
	}
	m.refreshComposeFooter()
	m.refreshComposeStatus()
	return m
}

type planStep struct {
	Text string
	Done bool
}

type questionPrompt struct {
	Prompt  string
	Options []string
	Input   textinput.Model
}

type modelCatalog struct {
	visible   bool
	rows      []modelCatalogRow
	loading   map[string]bool
	selection int
}

type modelCatalogRow struct {
	Display    string
	Provider   string
	ModelName  string
	Selectable bool
}

type blockRef struct {
	start  int
	length int
}

type streamingResponse struct {
	title  string
	style  lipgloss.Style
	block  blockRef
	buffer string
}

type responseStreamState struct {
	stream <-chan provider.StreamChunk
}

type responseChunkMsg struct {
	Text string
	Err  error
	Done bool
}

func initSession(opts Options) (history.Session, string) {
	if opts.ResumeID != "" {
		session, err := history.Get(opts.ResumeID)
		if err != nil {
			return history.Session{}, fmt.Sprintf("resume error: %v", err)
		}
		return session, fmt.Sprintf("Resumed chat %s", session.ID)
	}
	session, err := history.CreateSession(opts.ProjectPath)
	if err != nil {
		return history.Session{}, fmt.Sprintf("session init error: %v", err)
	}
	return session, fmt.Sprintf("New chat %s", session.ID)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, listenExecEvents(m.executor))
}

func listenExecEvents(exec *toolexec.Executor) tea.Cmd {
	if exec == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-exec.Events()
		if !ok {
			return nil
		}
		return execEventMsg{job: event.Job}
	}
}

type execEventMsg struct {
	job toolexec.Job
}

type modelFetchMsg struct {
	provider string
	models   []provider.Model
	err      error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyTab {
			if m.tryTabComplete(true) {
				return m, nil
			}
			m.cyclePlanMode()
			return m, nil
		}
		if msg.Type == tea.KeyShiftTab {
			if m.tryTabComplete(false) {
				return m, nil
			}
			m.cyclePlanMode()
			return m, nil
		}
		if m.question != nil {
			return m.updateQuestion(msg)
		}
		if msg.String() == "esc" && m.catalog.visible {
			m.catalog.visible = false
			return m, nil
		}
		if m.catalog.visible {
			switch msg.Type {
			case tea.KeyUp:
				if m.moveCatalogSelection(-1) {
					return m, nil
				}
			case tea.KeyDown:
				if m.moveCatalogSelection(1) {
					return m, nil
				}
			case tea.KeyEnter:
				if m.activateSelectedCatalogModel() {
					return m, nil
				}
			}
		}
		switch msg.String() {
		case "/":
			m.commandPalette.activate()
			return m, nil
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.commandPalette.visible {
				m.commandPalette.Reset()
			}
			if m.executor != nil && m.executor.CancelForeground() {
				m.statusLine = "Canceled foreground command."
				return m, nil
			}
			if m.pendingResponse != nil {
				m.finishResponseStream()
				m.messages = append(m.messages, "pfui: canceled response stream")
				return m, nil
			}
			m.compose.Reset()
			m.compose.Focus()
			m.recallMode = false
			m.refreshComposeStatus()
			return m, nil
		case "enter":
			if handled, cmd := m.processCommandPaletteKey(msg); handled {
				return m, cmd
			}
			return m.submitInput()
		case "ctrl+r":
			return m.handleReverseSearch()
		default:
			if m.recallMode && msg.String() != "ctrl+r" {
				m.recallMode = false
				m.refreshComposeStatus()
			}
			if handled, cmd := m.processCommandPaletteKey(msg); handled {
				return m, cmd
			}
			var cmd tea.Cmd
			m.compose, cmd = m.compose.Update(msg)
			return m, cmd
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.compose.SetWidth(msg.Width)
		return m, nil
	case spinner.TickMsg:
		if m.pendingResponse != nil {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			m.refreshComposeStatus()
			return m, cmd
		}
		return m, nil
	case execEventMsg:
		if msg.job.ID != "" {
			m.jobs[msg.job.ID] = msg.job
			m.recordJobEvent(msg.job)
		}
		return m, listenExecEvents(m.executor)
	case modelFetchMsg:
		if msg.err != nil {
			if m.catalog.visible {
				m.catalog.rows = append(m.catalog.rows, modelCatalogRow{
					Display: fmt.Sprintf("%s: error %v", msg.provider, msg.err),
				})
				delete(m.catalog.loading, msg.provider)
				m.ensureCatalogSelection()
			}
			m.messages = append(m.messages, fmt.Sprintf("pfui: %s error: %v", msg.provider, msg.err))
			return m, nil
		}
		if len(msg.models) == 0 {
			if m.catalog.visible {
				m.catalog.rows = append(m.catalog.rows, modelCatalogRow{
					Display: fmt.Sprintf("%s: no models match filter", msg.provider),
				})
				delete(m.catalog.loading, msg.provider)
				m.ensureCatalogSelection()
			}
			m.messages = append(m.messages, fmt.Sprintf("%s: no models match the current whitelist", msg.provider))
			return m, nil
		}
		for _, entry := range msg.models {
			caps := strings.Join(entry.Capabilities, ",")
			tags := summarizeTags(entry.Tags)
			if m.catalog.visible {
				m.catalog.rows = append(m.catalog.rows, modelCatalogRow{
					Display:    fmt.Sprintf("%s ▸ %s [%s]%s", msg.provider, entry.Name, caps, tags),
					Provider:   msg.provider,
					ModelName:  entry.Name,
					Selectable: true,
				})
			}
			m.messages = append(m.messages, fmt.Sprintf("%s ▸ %s [%s]%s", msg.provider, entry.Name, caps, tags))
		}
		if m.catalog.visible {
			delete(m.catalog.loading, msg.provider)
			m.ensureCatalogSelection()
		}
		return m, nil
	case responseChunkMsg:
		if m.pendingResponse == nil {
			return m, nil
		}
		if msg.Err != nil {
			m.messages = append(m.messages, fmt.Sprintf("pfui: %v", msg.Err))
			m.finishResponseStream()
			return m, nil
		}
		if msg.Text != "" {
			m.pendingResponse.buffer += msg.Text
			body := strings.Split(m.pendingResponse.buffer, "\n")
			m.replaceHistoryBlock(&m.pendingResponse.block, m.pendingResponse.title, body, m.pendingResponse.style)
		}
		if msg.Done {
			m.finishResponseStream()
			return m, nil
		}
		return m, m.nextResponseChunkCmd()
	default:
		var cmd tea.Cmd
		m.compose, cmd = m.compose.Update(msg)
		return m, cmd
	}
}

func (m model) View() string {
	paletteView := ""
	paletteLines := 0
	if m.commandPalette.visible {
		paletteView = m.commandPalette.View()
		paletteLines = countLines(paletteView)
	}
	catalogView := ""
	catalogLines := 0
	if m.catalog.visible {
		catalogView = renderModelCatalog(m.catalog)
		catalogLines = countLines(catalogView)
	}
	planView := ""
	planLines := 0
	if m.showPlan {
		planView = renderPlanDrawer(m.planSteps, m.cfg.Plan)
		planLines = countLines(planView)
	}
	questionView := ""
	questionLines := 0
	if m.question != nil {
		questionView = renderQuestionPrompt(m.question)
		questionLines = countLines(questionView)
	}
	composeView := m.compose.View()
	composeLines := countLines(composeView)
	jobLine := summarizeJobs(m.jobs)
	status := m.statusDisplay()
	modeBadge := m.modeBadge()
	dockHeight := 2 // separator + hint line
	if status != "" {
		dockHeight++
	}
	if modeBadge != "" {
		dockHeight++
	}
	if jobLine != "" {
		dockHeight++
	}
	dockHeight += paletteLines + catalogLines + planLines + questionLines + composeLines
	viewportHeight := m.height - dockHeight
	if viewportHeight < 3 {
		viewportHeight = 3
	}
	visible := lastLines(m.messages, viewportHeight)
	builder := strings.Builder{}
	for _, line := range visible {
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteString(strings.Repeat("─", max(10, m.width)))
	builder.WriteByte('\n')
	if status != "" {
		builder.WriteString(status)
		builder.WriteByte('\n')
	}
	if modeBadge != "" {
		builder.WriteString(modeBadge)
		builder.WriteByte('\n')
	}
	if paletteView != "" {
		builder.WriteString(paletteView)
	}
	if catalogView != "" {
		builder.WriteString(catalogView)
	}
	if planView != "" {
		builder.WriteString(planView)
	}
	if composeView != "" {
		builder.WriteString(composeView)
	}
	if questionView != "" {
		builder.WriteString(questionView)
		builder.WriteByte('\n')
	}
	if jobLine != "" {
		builder.WriteString(jobLine)
		builder.WriteByte('\n')
	}
	builder.WriteString("[enter] send  [esc] cancel/clear  [tab] cycle mode  [ctrl+r] reverse search  [/model] picker  [/jobs] list\n")
	return builder.String()
}

func (m model) submitInput() (tea.Model, tea.Cmd) {
	if m.question != nil {
		answer := strings.TrimSpace(m.question.Input.Value())
		if answer == "" {
			m.messages = append(m.messages, "pfui: answer cannot be empty")
			return m, nil
		}
		m.messages = append(m.messages, fmt.Sprintf("[answer] %s", answer))
		m.question = nil
		m.compose.Reset()
		m.compose.Focus()
		m.refreshComposeStatus()
		return m, nil
	}
	text := strings.TrimSpace(m.compose.Value())
	m.compose.Reset()
	if text == "" {
		return m, nil
	}
	if m.commandPalette.visible {
		m.commandPalette.Reset()
	}
	if strings.HasPrefix(text, "/") {
		return m.handleCommand(text)
	}
	if m.activeProvider == nil {
		if m.trySelectProvider(text) {
			m.refreshComposeFooter()
			return m, nil
		}
		m.messages = append(m.messages, providerPromptText(m.available))
		return m, nil
	}
	m.promptHistory = append(m.promptHistory, text)
	m.recallMode = false
	m.refreshComposeStatus()
	m.appendStyledHistoryBlock(fmt.Sprintf("you (%s)", providerLabel(m.activeProvider)), []string{text}, userBlockStyle)
	if m.session.ID != "" {
		if m.session.Title == "New chat" {
			m.session.Title = truncate(text, 60)
		}
		m.session.Summary = truncate(text, 120)
		if err := history.Save(m.session); err != nil {
			m.statusLine = fmt.Sprintf("history save error: %v", err)
		} else {
			m.statusLine = fmt.Sprintf("Updated %s at %s", m.session.ID, time.Now().Format(time.Kitchen))
		}
	}
	return m, m.beginResponseStream(text)
}

func (m model) handleReverseSearch() (tea.Model, tea.Cmd) {
	if len(m.promptHistory) == 0 {
		return m, nil
	}
	if !m.recallMode {
		m.recallMode = true
		m.recallPosition = len(m.promptHistory)
		m.refreshComposeStatus()
	}
	if m.recallPosition > 0 {
		m.recallPosition--
		m.compose.SetValue(m.promptHistory[m.recallPosition])
		m.compose.CursorEnd()
	}
	return m, nil
}

func (m model) handleCommand(text string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(text)
	cmd := strings.TrimPrefix(parts[0], "/")
	switch cmd {
	case "model":
		return m, m.showModelCatalog()
	case "jobs":
		m.handleJobsCommand(parts[1:])
	case "config":
		m.messages = append(m.messages, "pfui: run `pfui --configuration` (or /config soon) to open the wizard. This will clear scrollback.")
	case "resume":
		m.messages = append(m.messages, "pfui: use --resume or start pfui with --resume to pick a chat. In-TUI /resume picker is on the roadmap.")
	case "status":
		status := m.statusDisplay()
		if status == "" {
			status = "pfui: no status to report"
		}
		m.messages = append(m.messages, status)
	case "usage":
		m.messages = append(m.messages, "pfui: usage polling is not wired yet. Use /status for now.")
	case "plan":
		return m.handlePlanCommand(parts[1:])
	case "auto":
		m.setPlanMode(planModeAuto)
	case "off":
		m.setPlanMode(planModeOff)
	case "ask":
		m.handleAskCommand(parts[1:])
	case "help":
		m.messages = append(m.messages, "pfui commands: /model /plan /auto /off /provider /jobs /status /usage /config /resume /ask")
	case "provider":
		if len(parts) < 2 {
			m.messages = append(m.messages, providerPromptText(m.available))
			return m, nil
		}
		selection := strings.Join(parts[1:], " ")
		if !m.trySelectProvider(selection) {
			m.messages = append(m.messages, fmt.Sprintf("pfui: provider %q not recognized", selection))
		}
	default:
		m.messages = append(m.messages, fmt.Sprintf("pfui: unknown command %s", text))
	}
	return m, nil
}

func (m *model) processCommandPaletteKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if !m.commandPalette.visible && m.commandPalette.SelectedCommand == "" {
		return false, nil
	}
	var handled bool
	var cmd tea.Cmd
	if m.commandPalette.visible {
		handled, cmd = m.commandPalette.UpdateKey(msg)
	}
	if m.commandPalette.SelectedCommand != "" && !m.commandPalette.visible {
		selection := m.commandPalette.SelectedCommand
		m.compose.SetValue(selection + " ")
		m.compose.CursorEnd()
		m.commandPalette.Reset()
		handled = true
	}
	return handled, cmd
}

func (m *model) showModelCatalog() tea.Cmd {
	providers := m.providers.Providers()
	if len(providers) == 0 {
		m.catalog.visible = false
		m.messages = append(m.messages, "pfui: no providers configured. Use --configuration to add OpenAI or Claude accounts.")
		return nil
	}
	m.catalog.visible = true
	m.catalog.rows = nil
	m.catalog.selection = 0
	if m.catalog.loading == nil {
		m.catalog.loading = make(map[string]bool)
	}
	for k := range m.catalog.loading {
		delete(m.catalog.loading, k)
	}
	cmds := make([]tea.Cmd, 0, len(providers))
	for _, p := range providers {
		m.messages = append(m.messages, fmt.Sprintf("Fetching models from %s…", p.Name()))
		m.catalog.loading[p.Name()] = true
		cmds = append(cmds, fetchModelsCmd(p, buildWhitelistSet(m.providerWhitelist(p))))
	}
	return tea.Batch(cmds...)
}

func (m *model) ensureCatalogSelection() {
	if len(m.catalog.rows) == 0 {
		m.catalog.selection = 0
		return
	}
	if m.catalog.selection >= len(m.catalog.rows) {
		m.catalog.selection = len(m.catalog.rows) - 1
	}
	if m.catalog.rows[m.catalog.selection].Selectable {
		return
	}
	for i, row := range m.catalog.rows {
		if row.Selectable {
			m.catalog.selection = i
			return
		}
	}
}

func (m *model) moveCatalogSelection(delta int) bool {
	if !m.catalog.visible || len(m.catalog.rows) == 0 {
		return false
	}
	idx := m.catalog.selection
	for attempts := 0; attempts < len(m.catalog.rows); attempts++ {
		idx += delta
		if idx < 0 {
			idx = len(m.catalog.rows) - 1
		} else if idx >= len(m.catalog.rows) {
			idx = 0
		}
		if !m.catalog.rows[idx].Selectable {
			continue
		}
		m.catalog.selection = idx
		return true
	}
	return false
}

func (m *model) activateSelectedCatalogModel() bool {
	if !m.catalog.visible || len(m.catalog.rows) == 0 {
		return false
	}
	row := m.catalog.rows[m.catalog.selection]
	if !row.Selectable {
		return false
	}
	p := m.providerByName(row.Provider)
	if p == nil {
		m.messages = append(m.messages, fmt.Sprintf("pfui: provider %s not recognized", row.Provider))
		return true
	}
	m.activeProvider = p
	m.awaitingProvider = false
	m.defaultModel = row.ModelName
	message := fmt.Sprintf("Using %s via %s", defaultModelDisplay(row.ModelName), p.Name())
	m.messages = append(m.messages, message)
	m.statusLine = message
	m.catalog.visible = false
	m.refreshComposeFooter()
	return true
}

func (m *model) providerByName(name string) provider.Provider {
	for _, p := range m.available {
		if strings.EqualFold(p.Name(), name) {
			return p
		}
	}
	return nil
}

func fetchModelsCmd(p provider.Provider, whitelist map[string]struct{}) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		models, err := p.ListModels(ctx)
		if err != nil {
			return modelFetchMsg{provider: p.Name(), err: err}
		}
		filtered := filterModels(models, whitelist)
		return modelFetchMsg{provider: p.Name(), models: filtered}
	}
}

func (m *model) trySelectProvider(input string) bool {
	if len(m.available) == 0 {
		return false
	}
	query := strings.ToLower(strings.TrimSpace(input))
	if query == "" {
		return false
	}
	for _, p := range m.available {
		if strings.EqualFold(p.Name(), query) || strings.EqualFold(string(p.Kind()), query) {
			m.activeProvider = p
			m.awaitingProvider = false
			m.defaultModel = defaultModelFor(p)
			message := fmt.Sprintf("Using %s via %s", defaultModelDisplay(m.defaultModel), p.Name())
			m.messages = append(m.messages, message)
			m.statusLine = message
			m.refreshComposeFooter()
			return true
		}
	}
	return false
}

func providerPromptText(providers []provider.Provider) string {
	if len(providers) == 0 {
		return "No providers are currently enabled."
	}
	var names []string
	for _, p := range providers {
		names = append(names, fmt.Sprintf("%s (%s)", p.Name(), p.Kind()))
	}
	return fmt.Sprintf("Select a provider with /provider <name>: %s", strings.Join(names, ", "))
}

func defaultModelFor(p provider.Provider) string {
	switch p.Kind() {
	case provider.KindOpenAI:
		return "gpt-5.1-codex"
	case provider.KindAnthropic:
		return "claude-4.5-sonnet"
	default:
		return ""
	}
}

func defaultModelDisplay(model string) string {
	if model == "" {
		return "the provider default"
	}
	return model
}

func (m model) statusDisplay() string {
	return m.statusLine
}

func (m model) modeBadge() string {
	switch m.plan {
	case planModePlan:
		return planBadgeStyle.Render("PLAN")
	case planModeAuto:
		return autoBadgeStyle.Render("AUTO")
	default:
		return ""
	}
}

func providerLabel(p provider.Provider) string {
	if p == nil {
		return "unknown-provider"
	}
	return p.Name()
}

func (m model) providerWhitelist(p provider.Provider) []string {
	if p != nil && m.cfg.Models.ProviderWhitelist != nil {
		name := strings.ToLower(p.Name())
		if list, ok := m.cfg.Models.ProviderWhitelist[name]; ok && len(list) > 0 {
			return list
		}
		kind := strings.ToLower(string(p.Kind()))
		if list, ok := m.cfg.Models.ProviderWhitelist[kind]; ok && len(list) > 0 {
			return list
		}
	}
	return m.cfg.Models.Whitelist
}

func buildWhitelistSet(list []string) map[string]struct{} {
	if len(list) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(list))
	for _, model := range list {
		trimmed := strings.TrimSpace(model)
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return set
}

func filterModels(models []provider.Model, whitelist map[string]struct{}) []provider.Model {
	if len(models) == 0 {
		return nil
	}
	if len(whitelist) == 0 {
		return models
	}
	var filtered []provider.Model
	for _, model := range models {
		if _, ok := whitelist[model.Name]; ok {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func summarizeTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	var parts []string
	for k, v := range tags {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return fmt.Sprintf(" (%s)", strings.Join(parts, ","))
}

func lastLines(lines []string, n int) []string {
	if n >= len(lines) {
		return lines
	}
	return lines[len(lines)-n:]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit])
}

func safeString(s string) string {
	if s == "" {
		return "n/a"
	}
	return s
}

func (m *model) appendHistoryBlock(title string, body []string) {
	m.appendStyledHistoryBlock(title, body, lipgloss.NewStyle())
}

func (m *model) appendStyledHistoryBlock(title string, body []string, style lipgloss.Style) {
	m.appendStyledHistoryBlockRef(title, body, style)
}

func (m *model) appendStyledHistoryBlockRef(title string, body []string, style lipgloss.Style) blockRef {
	lines := historyBlockLines(title, body)
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	start := len(m.messages)
	m.messages = append(m.messages, lines...)
	return blockRef{start: start, length: len(lines)}
}

func (m *model) replaceHistoryBlock(ref *blockRef, title string, body []string, style lipgloss.Style) {
	if ref == nil {
		return
	}
	lines := historyBlockLines(title, body)
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	start := ref.start
	end := start + ref.length
	if start > len(m.messages) {
		start = len(m.messages)
	}
	if end > len(m.messages) {
		end = len(m.messages)
	}
	updated := make([]string, 0, len(m.messages)-(end-start)+len(lines))
	updated = append(updated, m.messages[:start]...)
	updated = append(updated, lines...)
	updated = append(updated, m.messages[end:]...)
	m.messages = updated
	ref.length = len(lines)
	ref.start = start
}

func historyBlockLines(title string, body []string) []string {
	folded := make([]string, 0, len(body))
	for _, line := range body {
		parts := strings.Split(line, "\n")
		folded = append(folded, parts...)
	}
	width := len(title)
	for _, line := range folded {
		if len(line) > width {
			width = len(line)
		}
	}
	border := strings.Repeat("─", width+2)
	lines := []string{fmt.Sprintf("┌─ %s", title)}
	for _, line := range folded {
		padding := width - len(line)
		if padding < 0 {
			padding = 0
		}
		lines = append(lines, fmt.Sprintf("│ %s%s", line, strings.Repeat(" ", padding)))
	}
	lines = append(lines, fmt.Sprintf("└%s", border))
	return lines
}

func buildSessionHeaderLines(session history.Session, projectPath string, planCfg config.PlanConfig, providers []provider.Provider, mode planMode) []string {
	project := session.Project
	if strings.TrimSpace(project) == "" {
		project = projectPath
	}
	providerLine := "providers: none configured"
	if len(providers) > 0 {
		var names []string
		for _, p := range providers {
			names = append(names, fmt.Sprintf("%s (%s)", p.Name(), p.Kind()))
		}
		providerLine = fmt.Sprintf("providers: %s", strings.Join(names, ", "))
	}
	planSummary := planStorageSummary(planCfg)
	return []string{
		providerLine,
		fmt.Sprintf("project: %s", safeString(project)),
		fmt.Sprintf("plan mode: %s (Tab cycles)", strings.ToUpper(string(mode))),
		fmt.Sprintf("plan storage: %s", planSummary),
		"commands: /plan /model /jobs /help",
	}
}

func planStorageSummary(planCfg config.PlanConfig) string {
	if !strings.EqualFold(planCfg.Storage, "file") {
		return "memory only"
	}
	path := strings.TrimSpace(planCfg.FilePath)
	if path == "" {
		path = "PLAN.md"
	}
	policy := "manual"
	if planCfg.AutoWrite {
		policy = "auto"
	}
	return fmt.Sprintf("file → %s (%s)", path, policy)
}

func (m *model) beginResponseStream(prompt string) tea.Cmd {
	if m.activeProvider == nil {
		return nil
	}
	if m.pendingResponse != nil {
		m.finishResponseStream()
	}
	title := fmt.Sprintf("pfui (%s/%s)", providerLabel(m.activeProvider), defaultModelDisplay(m.defaultModel))
	ref := m.appendStyledHistoryBlockRef(title, []string{"…"}, assistantBlockStyle)
	m.pendingResponse = &streamingResponse{title: title, style: assistantBlockStyle, block: ref}

	req := provider.ChatCompletionRequest{
		Model:    m.defaultModel,
		Messages: []provider.ChatMessage{{Role: "user", Content: prompt}},
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.pendingCancel = cancel
	stream, err := m.activeProvider.StreamChat(ctx, req)
	if err != nil {
		m.finishResponseStream()
		m.messages = append(m.messages, fmt.Sprintf("pfui: %v", err))
		return nil
	}
	m.responseStream = &responseStreamState{stream: stream}
	m.refreshComposeStatus()
	cmd := m.nextResponseChunkCmd()
	if cmd == nil {
		m.finishResponseStream()
		return nil
	}
	return tea.Batch(cmd, m.spinner.Tick)
}

func (m *model) nextResponseChunkCmd() tea.Cmd {
	if m.responseStream == nil {
		return nil
	}
	stream := m.responseStream.stream
	return func() tea.Msg {
		chunk, ok := <-stream
		if !ok {
			return responseChunkMsg{Done: true}
		}
		return responseChunkMsg{Text: chunk.Content, Err: chunk.Err, Done: chunk.Done}
	}
}

func (m *model) finishResponseStream() {
	if m.pendingCancel != nil {
		m.pendingCancel()
		m.pendingCancel = nil
	}
	m.pendingResponse = nil
	m.responseStream = nil
	m.refreshComposeStatus()
}

func summarizeJobs(jobs map[string]toolexec.Job) string {
	if len(jobs) == 0 {
		return ""
	}
	var running, success, failed int
	for _, job := range jobs {
		switch job.Status {
		case toolexec.JobSuccess:
			success++
		case toolexec.JobFailed:
			failed++
		default:
			running++
		}
	}
	if running == 0 && success == 0 && failed == 0 {
		return ""
	}
	return fmt.Sprintf("jobs: %d running · %d done · %d failed (/jobs)", running, success, failed)
}

func countLines(block string) int {
	if block == "" {
		return 0
	}
	trimmed := strings.TrimRight(block, "\n")
	if trimmed == "" {
		return 1
	}
	return strings.Count(trimmed, "\n") + 1
}

func (m *model) handleJobsCommand(args []string) {
	if len(args) >= 2 && strings.EqualFold(args[0], "cancel") {
		id := args[1]
		if m.executor != nil && m.executor.CancelJob(id) {
			m.messages = append(m.messages, fmt.Sprintf("pfui: canceling job %s", id))
		} else {
			m.messages = append(m.messages, fmt.Sprintf("pfui: job %s not found", id))
		}
		return
	}
	if len(m.jobs) == 0 {
		m.messages = append(m.messages, "pfui: no background jobs running.")
		return
	}
	ids := make([]string, 0, len(m.jobs))
	for id := range m.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		job := m.jobs[id]
		m.messages = append(m.messages, fmt.Sprintf("%s %s [%s] exit=%d", shortJobID(id), job.Command, strings.ToUpper(string(job.Status)), job.ExitCode))
	}
}

func (m *model) setPlanMode(mode planMode) {
	if m.plan == mode {
		m.messages = append(m.messages, fmt.Sprintf("pfui: already in %s mode", strings.ToUpper(string(mode))))
		return
	}
	m.plan = mode
	m.statusLine = fmt.Sprintf("Switched to %s mode", strings.ToUpper(string(mode)))
	m.messages = append(m.messages, fmt.Sprintf("pfui: switched to %s mode", strings.ToUpper(string(mode))))
	m.refreshComposeFooter()
}

func (m *model) cyclePlanMode() {
	switch m.plan {
	case planModePlan:
		m.setPlanMode(planModeAuto)
	case planModeAuto:
		m.setPlanMode(planModeOff)
	default:
		m.setPlanMode(planModePlan)
	}
}

func (m *model) recordJobEvent(job toolexec.Job) {
	prefix := fmt.Sprintf("[job %s]", shortJobID(job.ID))
	switch job.Status {
	case toolexec.JobRunning:
		m.messages = append(m.messages, fmt.Sprintf("%s started %s%s", prefix, job.Command, formatArgs(job.Args)))
	case toolexec.JobSuccess:
		m.messages = append(m.messages, fmt.Sprintf("%s completed (exit %d)", prefix, job.ExitCode))
	case toolexec.JobFailed:
		msg := fmt.Sprintf("%s failed (exit %d)", prefix, job.ExitCode)
		if job.Error != "" {
			msg += ": " + job.Error
		}
		m.messages = append(m.messages, msg)
	}
}

func (m *model) refreshComposeFooter() {
	project := strings.TrimSpace(m.session.Project)
	if project == "" {
		project = strings.TrimSpace(m.opts.ProjectPath)
	}
	var parts []string
	if project != "" {
		parts = append(parts, fmt.Sprintf("project %s", project))
	}
	if m.activeProvider != nil {
		parts = append(parts, fmt.Sprintf("provider %s (%s)", m.activeProvider.Name(), defaultModelDisplay(m.defaultModel)))
	} else if len(m.available) > 0 {
		parts = append(parts, "provider select with /provider")
	} else {
		parts = append(parts, "provider none configured")
	}
	parts = append(parts, fmt.Sprintf("plan %s", strings.ToUpper(string(m.plan))))
	parts = append(parts, fmt.Sprintf("plan storage %s", planStorageSummary(m.cfg.Plan)))
	if len(parts) == 0 {
		m.compose.SetInfoLine("")
		return
	}
	m.compose.SetInfoLine(strings.Join(parts, " · "))
}

func (m *model) refreshComposeStatus() {
	status := "esc to cancel · ctrl+r history"
	if m.recallMode {
		status = "reverse search ↑/↓ · enter to run"
	} else if m.pendingResponse != nil {
		status = fmt.Sprintf("%s generating… · esc to cancel", m.spinner.View())
	}
	m.compose.SetStatus(status, "? for shortcuts")
}

func shortJobID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func formatArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return " " + strings.Join(args, " ")
}

// TODO: replace with shared TUI components when the catalog drawer migrates to the
// reusable layout primitives (lipgloss/table, etc.).
func renderModelCatalog(c modelCatalog) string {
	var b strings.Builder
	b.WriteString("Models:\n")
	for provider := range c.loading {
		b.WriteString(fmt.Sprintf("  %s … loading\n", provider))
	}
	for i, row := range c.rows {
		prefix := "  "
		if i == c.selection {
			if row.Selectable {
				prefix = "> "
			} else {
				prefix = "* "
			}
		}
		b.WriteString(prefix + row.Display + "\n")
	}
	if len(c.loading) == 0 && len(c.rows) == 0 {
		b.WriteString("  No models found.\n")
	}
	b.WriteString("  [↑/↓] move  [enter] select  [esc] close /model drawer\n")
	return b.String()
}

func (m *model) tryTabComplete(forward bool) bool {
	value := m.compose.Value()
	trimmed := strings.TrimLeft(value, " \t")
	if !strings.HasPrefix(trimmed, "/") {
		return false
	}
	leftPad := value[:len(value)-len(trimmed)]
	cmdEnd := strings.IndexAny(trimmed, " \t\r\n")
	command := trimmed
	suffix := ""
	if cmdEnd >= 0 {
		command = trimmed[:cmdEnd]
		suffix = trimmed[cmdEnd:]
	}
	if command == "" {
		return false
	}
	m.commandPalette.setFilter(command)
	delta := 1
	if !forward {
		delta = -1
	}
	match := m.commandPalette.cycleSelection(delta)
	if match == "" {
		return true
	}
	m.compose.SetValue(leftPad + match + suffix)
	m.compose.CursorEnd()
	return true
}

func printResumeHint(sessionID, launchArgs string) {
	command := "pfui"
	if strings.TrimSpace(launchArgs) != "" {
		command = fmt.Sprintf("%s %s", command, strings.TrimSpace(launchArgs))
	}
	fmt.Printf("\nTo resume this chat later, run:\n  %s --resume %s\n\n", command, sessionID)
}

func (m model) planWritesToFile() bool {
	return strings.EqualFold(m.cfg.Plan.Storage, "file")
}

func (m model) resolvePlanPath(custom string) string {
	target := strings.TrimSpace(custom)
	if target == "" {
		target = strings.TrimSpace(m.cfg.Plan.FilePath)
		if target == "" {
			target = "PLAN.md"
		}
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	base := strings.TrimSpace(m.opts.ProjectPath)
	if base == "" {
		return filepath.Clean(target)
	}
	return filepath.Clean(filepath.Join(base, target))
}

func (m model) planDisplayPath(resolved string) string {
	base := strings.TrimSpace(m.opts.ProjectPath)
	if base != "" {
		if rel, err := filepath.Rel(base, resolved); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return rel
		}
	}
	return resolved
}

func (m *model) savePlanToFile(target string) (string, error) {
	path := strings.TrimSpace(target)
	if path == "" && !m.planWritesToFile() {
		return "", fmt.Errorf("plan storage is set to memory; run /plan save <path> to export")
	}
	resolved := m.resolvePlanPath(path)
	dir := filepath.Dir(resolved)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	var b strings.Builder
	b.WriteString("# Plan\n\n")
	if len(m.planSteps) == 0 {
		b.WriteString("_No steps yet_\n")
	} else {
		for i, step := range m.planSteps {
			box := "[ ]"
			if step.Done {
				box = "[x]"
			}
			b.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, box, step.Text))
		}
	}
	if err := os.WriteFile(resolved, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return resolved, nil
}

func (m *model) maybePersistPlan(reason string) {
	if !m.cfg.Plan.AutoWrite || !m.planWritesToFile() {
		return
	}
	if _, err := m.savePlanToFile(""); err != nil {
		m.messages = append(m.messages, fmt.Sprintf("pfui: plan auto-save error: %v", err))
		return
	}
	m.statusLine = fmt.Sprintf("Plan auto-saved (%s)", reason)
}

func renderPlanDrawer(steps []planStep, planCfg config.PlanConfig) string {
	var b strings.Builder
	b.WriteString("Plan:\n")
	for i, step := range steps {
		status := "[ ]"
		if step.Done {
			status = "[x]"
		}
		b.WriteString(fmt.Sprintf("  %d. %s %s\n", i+1, status, step.Text))
	}
	if len(steps) == 0 {
		b.WriteString("  (no steps yet — try /plan add)\n")
	}
	if strings.EqualFold(planCfg.Storage, "file") {
		path := strings.TrimSpace(planCfg.FilePath)
		if path == "" {
			path = "PLAN.md"
		}
		mode := "manual"
		if planCfg.AutoWrite {
			mode = "auto"
		}
		b.WriteString(fmt.Sprintf("  Plan file: %s (%s)\n", path, mode))
	}
	b.WriteString("  /plan save [path] writes the plan to disk\n")
	return b.String()
}

func (m *model) handlePlanCommand(args []string) (tea.Model, tea.Cmd) {
	if len(args) == 0 {
		m.showPlan = !m.showPlan
		state := "hidden"
		if m.showPlan {
			state = "visible"
		}
		m.messages = append(m.messages, fmt.Sprintf("pfui: plan drawer %s", state))
		return m, nil
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "add":
		text := strings.TrimSpace(strings.Join(args[1:], " "))
		if text == "" {
			m.messages = append(m.messages, "pfui: /plan add requires text")
			return m, nil
		}
		m.planSteps = append(m.planSteps, planStep{Text: text})
		m.showPlan = true
		m.messages = append(m.messages, fmt.Sprintf("pfui: added plan step %q", text))
		m.maybePersistPlan("step added")
	case "done":
		if len(args) < 2 {
			m.messages = append(m.messages, "pfui: /plan done <number>")
			return m, nil
		}
		idx, err := parseIndex(args[1], len(m.planSteps))
		if err != nil {
			m.messages = append(m.messages, fmt.Sprintf("pfui: %v", err))
			return m, nil
		}
		m.planSteps[idx].Done = true
		m.messages = append(m.messages, fmt.Sprintf("pfui: marked step %d complete", idx+1))
		m.maybePersistPlan("step updated")
	case "clear":
		m.planSteps = nil
		m.messages = append(m.messages, "pfui: cleared plan")
		m.maybePersistPlan("plan cleared")
	case "mode":
		if len(args) < 2 {
			m.messages = append(m.messages, "pfui: /plan mode <plan|auto|off>")
			return m, nil
		}
		switch strings.ToLower(args[1]) {
		case "plan":
			m.setPlanMode(planModePlan)
		case "auto":
			m.setPlanMode(planModeAuto)
		case "off":
			m.setPlanMode(planModeOff)
		default:
			m.messages = append(m.messages, "pfui: unknown plan mode")
		}
	case "show":
		m.showPlan = true
	case "hide":
		m.showPlan = false
	case "save":
		var manual string
		if len(args) > 1 {
			manual = strings.Join(args[1:], " ")
		}
		resolved, err := m.savePlanToFile(manual)
		if err != nil {
			m.messages = append(m.messages, fmt.Sprintf("pfui: %v", err))
		} else {
			display := m.planDisplayPath(resolved)
			m.messages = append(m.messages, fmt.Sprintf("pfui: plan saved to %s", display))
		}
	default:
		m.messages = append(m.messages, fmt.Sprintf("pfui: unknown /plan subcommand %s", sub))
	}
	return m, nil
}

func parseIndex(input string, total int) (int, error) {
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > total {
		return 0, fmt.Errorf("invalid step index")
	}
	return idx - 1, nil
}

func (m *model) handleAskCommand(args []string) {
	if len(args) == 0 {
		m.messages = append(m.messages, "pfui: /ask <question>? [option1|option2]")
		return
	}
	question := strings.Join(args, " ")
	parts := strings.Split(question, "|")
	prompt := strings.TrimSpace(parts[0])
	var options []string
	if len(parts) > 1 {
		for _, opt := range parts[1:] {
			opt = strings.TrimSpace(opt)
			if opt != "" {
				options = append(options, opt)
			}
		}
	}
	qi := textinput.New()
	qi.Placeholder = "Type answer or select option"
	qi.Focus()
	m.question = &questionPrompt{Prompt: prompt, Options: options, Input: qi}
	m.messages = append(m.messages, fmt.Sprintf("[question] %s", prompt))
}

func renderQuestionPrompt(q *questionPrompt) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Question: %s\n", q.Prompt))
	if len(q.Options) > 0 {
		for i, opt := range q.Options {
			b.WriteString(fmt.Sprintf("  %d) %s\n", i+1, opt))
		}
		b.WriteString("Type an option number or enter a custom response.\n")
	}
	b.WriteString(q.Input.View())
	return b.String()
}

func (m model) updateQuestion(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		m.question = nil
		m.messages = append(m.messages, "pfui: dismissed question")
		m.compose.Focus()
		m.refreshComposeStatus()
		return m, nil
	}
	if msg.Type == tea.KeyEnter {
		return m.submitInput()
	}
	if m.question == nil {
		return m, nil
	}
	if len(m.question.Options) > 0 && len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
		idx := int(msg.Runes[0] - '1')
		if idx < len(m.question.Options) {
			m.question.Input.SetValue(m.question.Options[idx])
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.question.Input, cmd = m.question.Input.Update(msg)
	return m, cmd
}
