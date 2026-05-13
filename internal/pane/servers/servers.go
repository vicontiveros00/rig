package servers

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/config"
	"github.com/vicontiveros00/rig/internal/llm"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
)

type serverStatus int

const (
	statusUnknown serverStatus = iota
	statusOnline
	statusOffline
	statusStopped
)

type serverEntry struct {
	name     string
	isModel  bool // true = model provider, false = mcp server
	status   serverStatus
	endpoint string
}

type formMode int

const (
	formNone formMode = iota
	formAddProvider
	formAddMCP
	formEditProvider
	formEditMCP
)

type healthResultMsg struct {
	name       string
	status     serverStatus
	httpStatus int
	err        error
}

type processStartedMsg struct {
	name string
	err  error
}

type processStoppedMsg struct {
	name string
	err  error
}

type managedProcess struct {
	cmd     *exec.Cmd
	running bool
}

type Servers struct {
	cfg       *config.Config
	providers map[string]llm.Provider
	width     int
	height    int

	entries  []serverEntry
	cursor   int
	statuses    map[string]serverStatus
	httpCodes   map[string]int
	procs       map[string]*managedProcess

	mode       formMode
	editTarget string
	inputs     []textinput.Model
	focusIdx   int

	confirmDelete bool
}

func New(cfg *config.Config, providers map[string]llm.Provider) pane.Pane {
	s := &Servers{
		cfg:       cfg,
		providers: providers,
		httpCodes: make(map[string]int),
		statuses:  make(map[string]serverStatus),
		procs:     make(map[string]*managedProcess),
	}
	s.rebuildEntries()
	return s
}

func (s *Servers) Name() string      { return "servers" }
func (s *Servers) ShortHelp() string  { return "manage servers" }
func (s *Servers) SetSize(w, h int)   { s.width = w; s.height = h }

func (s *Servers) Init() tea.Cmd {
	return s.checkAllHealth()
}

func (s *Servers) rebuildEntries() {
	s.entries = nil
	for name, pcfg := range s.cfg.Providers {
		s.entries = append(s.entries, serverEntry{
			name:     name,
			isModel:  true,
			endpoint: pcfg.Endpoint,
			status:   s.statuses[name],
		})
	}
	for name, mcfg := range s.cfg.MCPServers {
		s.entries = append(s.entries, serverEntry{
			name:     name,
			isModel:  false,
			endpoint: mcfg.Endpoint,
			status:   s.statuses[name],
		})
	}
}

func (s *Servers) checkAllHealth() tea.Cmd {
	var cmds []tea.Cmd
	for name, pcfg := range s.cfg.Providers {
		n, ep := name, pcfg.Endpoint
		cmds = append(cmds, func() tea.Msg {
			return checkEndpointHealth(n, ep)
		})
	}
	for name, mcfg := range s.cfg.MCPServers {
		n, ep, transport := name, mcfg.Endpoint, mcfg.Transport
		cmds = append(cmds, func() tea.Msg {
			if transport == "stdio" {
				return healthResultMsg{name: n, status: statusStopped}
			}
			return checkEndpointHealth(n, ep)
		})
	}
	return tea.Batch(cmds...)
}

var healthClient = &http.Client{
	Timeout: 5 * time.Second,
	Transport: &http.Transport{
		DisableKeepAlives: true,
	},
}

func checkEndpointHealth(name, endpoint string) healthResultMsg {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	url := strings.TrimSuffix(endpoint, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return healthResultMsg{name: name, status: statusOffline, err: err}
	}

	resp, err := healthClient.Do(req)
	if err != nil {
		return healthResultMsg{name: name, status: statusOffline, err: err}
	}
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return healthResultMsg{name: name, status: statusOffline, httpStatus: resp.StatusCode}
	}
	return healthResultMsg{name: name, status: statusOnline, httpStatus: resp.StatusCode}
}

func (s *Servers) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case healthResultMsg:
		s.statuses[msg.name] = msg.status
		if msg.httpStatus != 0 {
			s.httpCodes[msg.name] = msg.httpStatus
		}
		s.rebuildEntries()
		return s, nil

	case processStartedMsg:
		if msg.err == nil {
			s.statuses[msg.name] = statusOnline
		} else {
			s.statuses[msg.name] = statusOffline
		}
		s.rebuildEntries()
		return s, nil

	case processStoppedMsg:
		s.statuses[msg.name] = statusStopped
		s.rebuildEntries()
		return s, nil

	case tea.KeyMsg:
		if s.confirmDelete {
			return s.updateConfirmDelete(msg)
		}
		if s.mode != formNone {
			return s.updateForm(msg)
		}
		return s.updateList(msg)
	}

	return s, nil
}

func (s *Servers) updateList(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if s.cursor > 0 {
			s.cursor--
		}
	case "down", "j":
		if s.cursor < len(s.entries)-1 {
			s.cursor++
		}
	case "a":
		s.initForm(formAddProvider)
	case "m":
		s.initForm(formAddMCP)
	case "e":
		if len(s.entries) > 0 {
			entry := s.entries[s.cursor]
			s.editTarget = entry.name
			if entry.isModel {
				s.initForm(formEditProvider)
			} else {
				s.initForm(formEditMCP)
			}
		}
	case "d":
		if len(s.entries) > 0 {
			s.confirmDelete = true
		}
	case "t":
		if len(s.entries) > 0 {
			entry := s.entries[s.cursor]
			name, ep := entry.name, entry.endpoint
			if !entry.isModel {
				mcfg := s.cfg.MCPServers[name]
				if mcfg.Transport == "stdio" {
					return s, nil
				}
			}
			return s, func() tea.Msg {
				return checkEndpointHealth(name, ep)
			}
		}
	case "s":
		if len(s.entries) > 0 {
			return s, s.toggleProcess(s.entries[s.cursor])
		}
	}
	return s, nil
}

func (s *Servers) updateConfirmDelete(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "y":
		if s.cursor < len(s.entries) {
			entry := s.entries[s.cursor]
			if entry.isModel {
				delete(s.cfg.Providers, entry.name)
				delete(s.providers, entry.name)
			} else {
				delete(s.cfg.MCPServers, entry.name)
			}
			_ = s.cfg.SaveConfig()
			s.rebuildEntries()
			if s.cursor >= len(s.entries) && s.cursor > 0 {
				s.cursor--
			}
			s.confirmDelete = false
			return s, func() tea.Msg {
				return messages.ServersChangedMsg{
					Providers:  s.providers,
					MCPChanged: !entry.isModel,
				}
			}
		}
		s.confirmDelete = false
	case "n", "esc":
		s.confirmDelete = false
	}
	return s, nil
}

func (s *Servers) initForm(mode formMode) {
	s.mode = mode
	switch mode {
	case formAddProvider, formEditProvider:
		s.inputs = make([]textinput.Model, 3)
		s.inputs[0] = textinput.New()
		s.inputs[0].Placeholder = "name (e.g. openai, ollama)"
		s.inputs[0].Focus()
		s.inputs[1] = textinput.New()
		s.inputs[1].Placeholder = "endpoint (e.g. http://localhost:11434/v1)"
		s.inputs[2] = textinput.New()
		s.inputs[2].Placeholder = "api key (leave empty if none)"

		if mode == formEditProvider {
			pcfg := s.cfg.Providers[s.editTarget]
			s.inputs[0].SetValue(s.editTarget)
			s.inputs[1].SetValue(pcfg.Endpoint)
			s.inputs[2].SetValue(pcfg.APIKey)
		}

	case formAddMCP, formEditMCP:
		s.inputs = make([]textinput.Model, 4)
		s.inputs[0] = textinput.New()
		s.inputs[0].Placeholder = "name (e.g. discord-mcp)"
		s.inputs[0].Focus()
		s.inputs[1] = textinput.New()
		s.inputs[1].Placeholder = "endpoint (e.g. http://localhost:8080)"
		s.inputs[2] = textinput.New()
		s.inputs[2].Placeholder = "api key (leave empty if none)"
		s.inputs[3] = textinput.New()
		s.inputs[3].Placeholder = "transport: sse or stdio"

		if mode == formEditMCP {
			mcfg := s.cfg.MCPServers[s.editTarget]
			s.inputs[0].SetValue(s.editTarget)
			s.inputs[1].SetValue(mcfg.Endpoint)
			s.inputs[2].SetValue(mcfg.APIKey)
			s.inputs[3].SetValue(mcfg.Transport)
		}
	}
	s.focusIdx = 0
}

func (s *Servers) updateForm(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		s.mode = formNone
		s.inputs = nil
		return s, nil
	case "down", "ctrl+n":
		s.focusIdx = (s.focusIdx + 1) % len(s.inputs)
		s.syncFormFocus()
		return s, nil
	case "up", "ctrl+p":
		s.focusIdx = (s.focusIdx - 1 + len(s.inputs)) % len(s.inputs)
		s.syncFormFocus()
		return s, nil
	case "enter":
		return s.saveForm()
	}

	var cmd tea.Cmd
	s.inputs[s.focusIdx], cmd = s.inputs[s.focusIdx].Update(msg)
	return s, cmd
}

func (s *Servers) syncFormFocus() {
	for i := range s.inputs {
		if i == s.focusIdx {
			s.inputs[i].Focus()
		} else {
			s.inputs[i].Blur()
		}
	}
}

func (s *Servers) saveForm() (pane.Pane, tea.Cmd) {
	switch s.mode {
	case formAddProvider, formEditProvider:
		name := strings.TrimSpace(s.inputs[0].Value())
		endpoint := strings.TrimSpace(s.inputs[1].Value())
		apiKey := strings.TrimSpace(s.inputs[2].Value())
		if name == "" || endpoint == "" {
			s.mode = formNone
			return s, nil
		}

		if s.mode == formEditProvider && s.editTarget != name {
			delete(s.cfg.Providers, s.editTarget)
			delete(s.providers, s.editTarget)
		}

		pcfg := config.ProviderConfig{
			Endpoint: endpoint,
			APIKey:   apiKey,
			Type:     inferProviderType(endpoint),
		}
		if s.cfg.Providers == nil {
			s.cfg.Providers = make(map[string]config.ProviderConfig)
		}
		s.cfg.Providers[name] = pcfg

		p, err := llm.NewProvider(name, pcfg)
		if err == nil {
			s.providers[name] = p
		}

		_ = s.cfg.SaveConfig()
		s.mode = formNone
		s.inputs = nil
		s.rebuildEntries()
		return s, func() tea.Msg {
			return messages.ServersChangedMsg{Providers: s.providers}
		}

	case formAddMCP, formEditMCP:
		name := strings.TrimSpace(s.inputs[0].Value())
		endpoint := strings.TrimSpace(s.inputs[1].Value())
		apiKey := strings.TrimSpace(s.inputs[2].Value())
		transport := strings.TrimSpace(s.inputs[3].Value())
		if name == "" || endpoint == "" {
			s.mode = formNone
			return s, nil
		}
		if transport == "" {
			transport = "sse"
		}

		if s.mode == formEditMCP && s.editTarget != name {
			delete(s.cfg.MCPServers, s.editTarget)
		}

		if s.cfg.MCPServers == nil {
			s.cfg.MCPServers = make(map[string]config.MCPServerConfig)
		}
		s.cfg.MCPServers[name] = config.MCPServerConfig{
			Endpoint:  endpoint,
			APIKey:    apiKey,
			Transport: transport,
		}

		_ = s.cfg.SaveConfig()
		s.mode = formNone
		s.inputs = nil
		s.rebuildEntries()
		return s, func() tea.Msg {
			return messages.ServersChangedMsg{Providers: s.providers, MCPChanged: true}
		}
	}

	s.mode = formNone
	return s, nil
}

func (s *Servers) toggleProcess(entry serverEntry) tea.Cmd {
	name := entry.name
	proc, exists := s.procs[name]

	if exists && proc.running {
		return func() tea.Msg {
			if proc.cmd != nil && proc.cmd.Process != nil {
				_ = proc.cmd.Process.Signal(syscall.SIGTERM)
				done := make(chan error, 1)
				go func() { done <- proc.cmd.Wait() }()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					_ = proc.cmd.Process.Kill()
				}
			}
			proc.running = false
			return processStoppedMsg{name: name}
		}
	}

	binary := resolveServerBinary(name)
	if binary == "" {
		return nil
	}

	return func() tea.Msg {
		cmd := exec.Command(binary, "serve")
		if err := cmd.Start(); err != nil {
			return processStartedMsg{name: name, err: err}
		}
		s.procs[name] = &managedProcess{cmd: cmd, running: true}
		time.Sleep(2 * time.Second)
		return processStartedMsg{name: name}
	}
}

func resolveServerBinary(name string) string {
	switch name {
	case "ollama":
		if p, err := exec.LookPath("ollama"); err == nil {
			return p
		}
	}
	return ""
}

func inferProviderType(endpoint string) string {
	lower := strings.ToLower(endpoint)
	if strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1") {
		return "local"
	}
	return "cloud"
}

func (s *Servers) View() string {
	if s.width == 0 {
		return ""
	}

	if s.mode != formNone {
		return s.viewForm()
	}

	var b strings.Builder

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#8B5CF6"))

	b.WriteString(headerStyle.Render("── model providers ──"))
	b.WriteString("\n\n")

	idx := 0
	for i, entry := range s.entries {
		if !entry.isModel {
			continue
		}
		line := s.renderEntry(entry, i)
		if i == s.cursor {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Width(s.width - 2).
				Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
		idx++
	}

	b.WriteString("\n")
	b.WriteString(headerStyle.Render("── mcp servers ──"))
	b.WriteString("\n\n")

	for i, entry := range s.entries {
		if entry.isModel {
			continue
		}
		line := s.renderEntry(entry, i)
		if i == s.cursor {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Width(s.width - 2).
				Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if s.confirmDelete && s.cursor < len(s.entries) {
		b.WriteString("\n")
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
		b.WriteString(warn.Render(fmt.Sprintf("  delete %q? (y/n)", s.entries[s.cursor].name)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  a = add provider  m = add mcp  e = edit  d = delete  t = test  s = start/stop"))

	return b.String()
}

func (s *Servers) renderEntry(entry serverEntry, _ int) string {
	nameStyle := lipgloss.NewStyle().Width(18).Foreground(lipgloss.Color("#E5E7EB"))
	epStyle := lipgloss.NewStyle().Width(s.width - 40).Foreground(lipgloss.Color("#9CA3AF"))

	ep := entry.endpoint
	maxEP := s.width - 40
	if maxEP < 10 {
		maxEP = 10
	}
	if len(ep) > maxEP {
		ep = ep[:maxEP-1] + "…"
	}

	status := s.statuses[entry.name]
	code := s.httpCodes[entry.name]
	var statusStr string
	switch status {
	case statusOnline:
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("● online")
	case statusOffline:
		label := "○ offline"
		if code != 0 {
			label = fmt.Sprintf("○ offline (%d)", code)
		}
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Render(label)
	case statusStopped:
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("○ stopped")
	default:
		statusStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("? unknown")
	}

	return fmt.Sprintf("  %s %s %s",
		nameStyle.Render(entry.name),
		epStyle.Render(ep),
		statusStr,
	)
}

func (s *Servers) viewForm() string {
	var b strings.Builder

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))

	var title string
	switch s.mode {
	case formAddProvider:
		title = "add model provider"
	case formEditProvider:
		title = "edit model provider"
	case formAddMCP:
		title = "add mcp server"
	case formEditMCP:
		title = "edit mcp server"
	}

	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	labels := []string{"name", "endpoint", "api key"}
	if s.mode == formAddMCP || s.mode == formEditMCP {
		labels = append(labels, "transport")
	}

	labelStyle := lipgloss.NewStyle().Width(12).Foreground(lipgloss.Color("#9CA3AF"))

	for i, input := range s.inputs {
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label+":"), input.View()))
	}

	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  ↑/↓ = next field  enter = save  esc = cancel"))

	return b.String()
}
