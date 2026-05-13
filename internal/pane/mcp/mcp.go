package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/config"
	mcpclient "github.com/vicontiveros00/rig/internal/mcp"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
)

type entryKind int

const (
	entryTool entryKind = iota
	entryResource
)

type entry struct {
	server      string
	kind        entryKind
	tool        mcpclient.Tool
	resource    mcpclient.Resource
	displayName string
	description string
}

type connectedMsg struct {
	server string
	tools  []mcpclient.Tool
	resources []mcpclient.Resource
	err    error
}

type toolResultMsg struct {
	result mcpclient.ToolResult
	err    error
}

type resourceReadMsg struct {
	content string
	err     error
}

type MCP struct {
	cfg     *config.Config
	clients map[string]*mcpclient.Client
	tools   map[string][]mcpclient.Tool
	resources map[string][]mcpclient.Resource
	errors  map[string]error

	entries []entry
	cursor  int
	filter  string
	filterInput textinput.Model
	filterActive bool

	invokeMode   bool
	invokeTarget entry
	argsInput    textinput.Model
	invokeVP     viewport.Model
	result       string
	resultErr    bool
	resultReady  bool
	loading      bool

	width  int
	height int
}

func New(cfg *config.Config) pane.Pane {
	fi := textinput.New()
	fi.Placeholder = "filter..."

	return &MCP{
		cfg:       cfg,
		clients:   make(map[string]*mcpclient.Client),
		tools:     make(map[string][]mcpclient.Tool),
		resources: make(map[string][]mcpclient.Resource),
		errors:    make(map[string]error),
		filterInput: fi,
	}
}

func (m *MCP) Name() string      { return "mcp" }
func (m *MCP) ShortHelp() string  { return "tools & resources" }
func (m *MCP) SetSize(w, h int)   { m.width = w; m.height = h }

func (m *MCP) Init() tea.Cmd {
	return m.connectAll()
}

func (m *MCP) connectAll() tea.Cmd {
	if len(m.cfg.MCPServers) == 0 {
		return nil
	}
	var cmds []tea.Cmd
	for name, scfg := range m.cfg.MCPServers {
		n, endpoint, apiKey, transport := name, scfg.Endpoint, scfg.APIKey, scfg.Transport
		cmds = append(cmds, func() tea.Msg {
			client := mcpclient.NewClient(endpoint, apiKey, transport)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			if err := client.Connect(ctx); err != nil {
				return connectedMsg{server: n, err: err}
			}

			tools, _ := client.ListTools(ctx)
			resources, _ := client.ListResources(ctx)

			return connectedMsg{
				server:    n,
				tools:     tools,
				resources: resources,
			}
		})
	}
	return tea.Batch(cmds...)
}

func (m *MCP) rebuildEntries() {
	m.entries = nil
	for server, tools := range m.tools {
		for _, t := range tools {
			e := entry{
				server:      server,
				kind:        entryTool,
				tool:        t,
				displayName: t.Name,
				description: t.Description,
			}
			if m.matchesFilter(e) {
				m.entries = append(m.entries, e)
			}
		}
	}
	for server, resources := range m.resources {
		for _, r := range resources {
			e := entry{
				server:      server,
				kind:        entryResource,
				resource:    r,
				displayName: r.URI,
				description: r.Description,
			}
			if m.matchesFilter(e) {
				m.entries = append(m.entries, e)
			}
		}
	}
}

func (m *MCP) matchesFilter(e entry) bool {
	if m.filter == "" {
		return true
	}
	lower := strings.ToLower(m.filter)
	return strings.Contains(strings.ToLower(e.displayName), lower) ||
		strings.Contains(strings.ToLower(e.description), lower) ||
		strings.Contains(strings.ToLower(e.server), lower)
}

func (m *MCP) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case connectedMsg:
		if msg.err != nil {
			m.errors[msg.server] = msg.err
		} else {
			delete(m.errors, msg.server)
			client := mcpclient.NewClient(
				m.cfg.MCPServers[msg.server].Endpoint,
				m.cfg.MCPServers[msg.server].APIKey,
				m.cfg.MCPServers[msg.server].Transport,
			)
			m.clients[msg.server] = client
			m.tools[msg.server] = msg.tools
			m.resources[msg.server] = msg.resources
		}
		m.rebuildEntries()
		return m, nil

	case toolResultMsg:
		m.loading = false
		if msg.err != nil {
			m.result = msg.err.Error()
			m.resultErr = true
		} else {
			m.result = msg.result.Content
			m.resultErr = msg.result.IsError
		}
		m.initResultViewport()
		return m, nil

	case resourceReadMsg:
		m.loading = false
		if msg.err != nil {
			m.result = msg.err.Error()
			m.resultErr = true
		} else {
			m.result = msg.content
			m.resultErr = false
		}
		m.initResultViewport()
		return m, nil

	case messages.ServersChangedMsg:
		if msg.MCPChanged {
			return m, m.connectAll()
		}
		return m, nil

	case tea.KeyMsg:
		if m.filterActive {
			return m.updateFilter(msg)
		}
		if m.invokeMode {
			return m.updateInvoke(msg)
		}
		return m.updateList(msg)
	}

	return m, nil
}

func (m *MCP) updateList(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.entries)-1 {
			m.cursor++
		}
	case "enter":
		if len(m.entries) > 0 {
			e := m.entries[m.cursor]
			if e.kind == entryTool {
				m.invokeMode = true
				m.invokeTarget = e
				m.argsInput = textinput.New()
				m.argsInput.Placeholder = schemaPlaceholder(e.tool.InputSchema)
				m.argsInput.Focus()
				m.argsInput.Width = m.width - 20
				m.result = ""
				m.resultErr = false
			} else {
				m.invokeMode = true
				m.invokeTarget = e
				m.loading = true
				m.result = ""
				return m, m.readResource(e)
			}
		}
	case "r":
		m.tools = make(map[string][]mcpclient.Tool)
		m.resources = make(map[string][]mcpclient.Resource)
		m.errors = make(map[string]error)
		m.entries = nil
		return m, m.connectAll()
	case "/":
		m.filterActive = true
		m.filterInput.Focus()
	}
	return m, nil
}

func (m *MCP) updateFilter(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filterActive = false
		m.filterInput.Blur()
		m.filter = ""
		m.filterInput.SetValue("")
		m.rebuildEntries()
		return m, nil
	case "enter":
		m.filterActive = false
		m.filterInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	m.filter = m.filterInput.Value()
	m.rebuildEntries()
	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
	return m, cmd
}

func (m *MCP) updateInvoke(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.invokeMode = false
		m.result = ""
		m.resultErr = false
		m.resultReady = false
		return m, nil
	case "enter":
		if m.invokeTarget.kind == entryTool && !m.loading {
			m.loading = true
			m.resultReady = false
			return m, m.callTool()
		}
		return m, nil
	}

	if m.resultReady {
		var cmd tea.Cmd
		m.invokeVP, cmd = m.invokeVP.Update(msg)
		return m, cmd
	}

	if m.invokeTarget.kind == entryTool {
		var cmd tea.Cmd
		m.argsInput, cmd = m.argsInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *MCP) callTool() tea.Cmd {
	server := m.invokeTarget.server
	toolName := m.invokeTarget.tool.Name
	argsStr := strings.TrimSpace(m.argsInput.Value())

	var args map[string]any
	if argsStr != "" {
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			return func() tea.Msg {
				return toolResultMsg{err: fmt.Errorf("invalid json: %w", err)}
			}
		}
	}

	scfg := m.cfg.MCPServers[server]
	return func() tea.Msg {
		client := mcpclient.NewClient(scfg.Endpoint, scfg.APIKey, scfg.Transport)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := client.Connect(ctx); err != nil {
			return toolResultMsg{err: err}
		}
		result, err := client.CallTool(ctx, toolName, args)
		return toolResultMsg{result: result, err: err}
	}
}

func (m *MCP) readResource(e entry) tea.Cmd {
	server := e.server
	uri := e.resource.URI

	scfg := m.cfg.MCPServers[server]
	return func() tea.Msg {
		client := mcpclient.NewClient(scfg.Endpoint, scfg.APIKey, scfg.Transport)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := client.Connect(ctx); err != nil {
			return resourceReadMsg{err: err}
		}
		content, err := client.ReadResource(ctx, uri)
		return resourceReadMsg{content: content, err: err}
	}
}

func (m *MCP) View() string {
	if m.width == 0 {
		return ""
	}

	if m.invokeMode {
		return m.viewInvoke()
	}
	return m.viewList()
}

func (m *MCP) viewList() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))

	if m.filterActive || m.filter != "" {
		b.WriteString(fmt.Sprintf("  filter: %s\n\n", m.filterInput.View()))
	}

	if len(m.cfg.MCPServers) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(dim.Render("  no mcp servers configured — add them in the servers pane"))
		return b.String()
	}

	// Group entries by server
	serverOrder := make([]string, 0)
	seen := make(map[string]bool)
	for name := range m.cfg.MCPServers {
		if !seen[name] {
			serverOrder = append(serverOrder, name)
			seen[name] = true
		}
	}

	globalIdx := 0
	for _, server := range serverOrder {
		status := "connected"
		if err, ok := m.errors[server]; ok {
			status = fmt.Sprintf("error: %s", err)
		} else if _, ok := m.tools[server]; !ok {
			status = "connecting..."
		}

		var statusRender string
		if _, ok := m.errors[server]; ok {
			statusRender = errStyle.Render(fmt.Sprintf("(%s)", status))
		} else {
			statusRender = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render(fmt.Sprintf("(%s)", status))
		}

		b.WriteString(headerStyle.Render(fmt.Sprintf("── %s ", server)))
		b.WriteString(statusRender)
		b.WriteString("\n\n")

		hasEntries := false
		for _, e := range m.entries {
			if e.server != server {
				continue
			}
			hasEntries = true
			line := m.renderEntry(e, globalIdx)
			if globalIdx == m.cursor {
				line = lipgloss.NewStyle().
					Background(lipgloss.Color("#1E1B4B")).
					Width(m.width - 2).
					Render(line)
			}
			b.WriteString(line)
			b.WriteString("\n")
			globalIdx++
		}

		if !hasEntries {
			if _, ok := m.errors[server]; !ok {
				dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
				b.WriteString(dim.Render("    (no tools or resources)"))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  r = refresh  enter = invoke/read  / = filter"))

	return b.String()
}

func (m *MCP) renderEntry(e entry, _ int) string {
	nameStyle := lipgloss.NewStyle().Width(22).Foreground(lipgloss.Color("#E5E7EB"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))

	var kindTag string
	switch e.kind {
	case entryTool:
		kindTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6")).Render("tool")
	case entryResource:
		kindTag = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("res ")
	}

	desc := e.description
	maxDesc := m.width - 38
	if maxDesc < 10 {
		maxDesc = 10
	}
	if len(desc) > maxDesc {
		desc = desc[:maxDesc-1] + "…"
	}

	return fmt.Sprintf("  %s %s %s", kindTag, nameStyle.Render(e.displayName), descStyle.Render(desc))
}

func (m *MCP) viewInvoke() string {
	if m.resultReady {
		var b strings.Builder
		b.WriteString(m.invokeVP.View())
		b.WriteString("\n")
		help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(help.Render(fmt.Sprintf("  %d%% — ↑/↓ scroll  enter = re-run  esc = back", int(m.invokeVP.ScrollPercent()*100))))
		return b.String()
	}

	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))

	e := m.invokeTarget
	if e.kind == entryTool {
		b.WriteString(headerStyle.Render(fmt.Sprintf("invoke: %s", e.tool.Name)))
		b.WriteString("\n")
		b.WriteString(descStyle.Render(fmt.Sprintf("  %s", e.tool.Description)))
		b.WriteString("\n\n")

		if params := schemaParams(e.tool.InputSchema); params != "" {
			paramStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
			b.WriteString(paramStyle.Render(params))
			b.WriteString("\n\n")
		}

		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		b.WriteString(labelStyle.Render("  args (json):"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %s", m.argsInput.View()))
		b.WriteString("\n")
	} else {
		b.WriteString(headerStyle.Render(fmt.Sprintf("resource: %s", e.resource.URI)))
		b.WriteString("\n")
		if e.resource.Description != "" {
			b.WriteString(descStyle.Render(fmt.Sprintf("  %s", e.resource.Description)))
			b.WriteString("\n")
		}
	}

	if m.loading {
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("  loading..."))
	}

	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	if e.kind == entryTool {
		b.WriteString(help.Render("  enter = execute  esc = back"))
	} else {
		b.WriteString(help.Render("  esc = back"))
	}

	return b.String()
}

func (m *MCP) buildInvokeContent(resultContent string) string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	descStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))

	e := m.invokeTarget
	if e.kind == entryTool {
		b.WriteString(headerStyle.Render(fmt.Sprintf("invoke: %s", e.tool.Name)))
		b.WriteString("\n")
		b.WriteString(descStyle.Render(fmt.Sprintf("  %s", e.tool.Description)))
		b.WriteString("\n\n")

		if params := schemaParams(e.tool.InputSchema); params != "" {
			paramStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
			b.WriteString(paramStyle.Render(params))
			b.WriteString("\n\n")
		}

		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		b.WriteString(labelStyle.Render(fmt.Sprintf("  args: %s", m.argsInput.Value())))
		b.WriteString("\n")
	} else {
		b.WriteString(headerStyle.Render(fmt.Sprintf("resource: %s", e.resource.URI)))
		b.WriteString("\n")
		if e.resource.Description != "" {
			b.WriteString(descStyle.Render(fmt.Sprintf("  %s", e.resource.Description)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#9CA3AF")).Render("  result:"))
	b.WriteString("\n")
	b.WriteString(resultContent)

	return b.String()
}

func (m *MCP) initResultViewport() {
	resultStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	if m.resultErr {
		resultStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	}

	var styled strings.Builder
	for _, line := range strings.Split(m.result, "\n") {
		styled.WriteString(resultStyle.Render("  " + line))
		styled.WriteString("\n")
	}

	vpHeight := m.height - 6
	if vpHeight < 3 {
		vpHeight = 3
	}
	m.invokeVP = viewport.New(m.width-2, vpHeight)
	m.invokeVP.SetContent(m.buildInvokeContent(styled.String()))
	m.resultReady = true
}

func schemaPlaceholder(schema map[string]any) string {
	props := schemaProperties(schema)
	if len(props) == 0 {
		return "{}"
	}
	var parts []string
	for _, kv := range props {
		parts = append(parts, fmt.Sprintf(`"%s": ""`, kv[0]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func schemaParams(schema map[string]any) string {
	props := schemaProperties(schema)
	if len(props) == 0 {
		return ""
	}

	required := make(map[string]bool)
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required[s] = true
			}
		}
	}

	var lines []string
	for _, kv := range props {
		name, typ := kv[0], kv[1]
		marker := "optional"
		if required[name] {
			marker = "required"
		}
		lines = append(lines, fmt.Sprintf("    %s (%s, %s)", name, typ, marker))
	}
	return "  parameters:\n" + strings.Join(lines, "\n")
}

func schemaProperties(schema map[string]any) [][2]string {
	if schema == nil {
		return nil
	}
	propsRaw, ok := schema["properties"]
	if !ok {
		return nil
	}
	propsMap, ok := propsRaw.(map[string]any)
	if !ok {
		return nil
	}
	var result [][2]string
	for name, v := range propsMap {
		typ := "string"
		if propMap, ok := v.(map[string]any); ok {
			if t, ok := propMap["type"].(string); ok {
				typ = t
			}
		}
		result = append(result, [2]string{name, typ})
	}
	return result
}
