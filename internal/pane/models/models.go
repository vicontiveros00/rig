package models

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/config"
	"github.com/vicontiveros00/rig/internal/llm"
	riglog "github.com/vicontiveros00/rig/internal/log"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
)

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7C3AED"))

	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#7C3AED")).
			Padding(0, 1)

	normalStyle = lipgloss.NewStyle().
			Padding(0, 1)

	activeMarker = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#10B981")).
		Bold(true).
		Render(" (active)")

	errStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EF4444")).
			Italic(true)

	hintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#6B7280"))
)

type modelsLoadedMsg struct {
	provider string
	models   []string
	err      error
}

type modelEntry struct {
	provider string
	model    string
}

type Pane struct {
	providers      map[string]llm.Provider
	models         map[string][]string
	errors         map[string]error
	flat           []modelEntry
	cursor         int
	filter         textinput.Model
	filtering      bool
	viewport       viewport.Model
	spinner        spinner.Model
	loading        int
	activeProvider string
	activeModel    string
	cfg            *config.Config
	width          int
	height         int
	lastRefresh    time.Time
}

func New(providers map[string]llm.Provider, cfg *config.Config, activeProvider, activeModel string) pane.Pane {
	ti := textinput.New()
	ti.Placeholder = "filter models..."
	ti.CharLimit = 100

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	p := &Pane{
		providers:      providers,
		models:         make(map[string][]string),
		errors:         make(map[string]error),
		filter:         ti,
		spinner:        sp,
		activeProvider: activeProvider,
		activeModel:    activeModel,
		cfg:            cfg,
	}

	if cfg.DiscoveredModels != nil {
		for name, ms := range cfg.DiscoveredModels {
			p.models[name] = ms
		}
		p.rebuildFlat()
	}

	return p
}

func (p *Pane) Name() string      { return "models" }
func (p *Pane) ShortHelp() string { return "discover and switch models" }

func (p *Pane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.filter.Width = width - 4
	p.viewport.Width = width
	p.viewport.Height = height - 2
	p.updateViewport()
}

func (p *Pane) Init() tea.Cmd {
	return tea.Batch(p.discoverAll()...)
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case modelsLoadedMsg:
		p.loading--
		if msg.err != nil {
			riglog.Error("models: %s discovery failed: %v", msg.provider, msg.err)
			p.errors[msg.provider] = msg.err
		} else {
			riglog.Info("models: %s discovered %d models", msg.provider, len(msg.models))
			delete(p.errors, msg.provider)
			p.models[msg.provider] = msg.models
		}
		p.rebuildFlat()
		if p.loading <= 0 {
			p.loading = 0
			p.lastRefresh = time.Now()
			p.persistModels()
		}
		p.updateViewport()
		return p, nil

	case spinner.TickMsg:
		if p.loading > 0 {
			var cmd tea.Cmd
			p.spinner, cmd = p.spinner.Update(msg)
			return p, cmd
		}
		return p, nil

	case tea.KeyMsg:
		if p.filtering {
			switch msg.String() {
			case "esc":
				p.filtering = false
				p.filter.Reset()
				p.filter.Blur()
				p.rebuildFlat()
				p.cursor = 0
				p.updateViewport()
				return p, nil
			case "enter":
				p.filtering = false
				p.filter.Blur()
				return p, nil
			default:
				var cmd tea.Cmd
				p.filter, cmd = p.filter.Update(msg)
				p.rebuildFlat()
				p.cursor = 0
				p.updateViewport()
				return p, cmd
			}
		}

		switch msg.String() {
		case "/":
			p.filtering = true
			p.filter.Focus()
			return p, textinput.Blink
		case "r":
			cmds := p.discoverAll()
			return p, tea.Batch(cmds...)
		case "up", "k":
			if p.cursor > 0 {
				p.cursor--
				p.scrollToCursor()
			}
			p.updateViewport()
			return p, nil
		case "down", "j":
			if p.cursor < len(p.flat)-1 {
				p.cursor++
				p.scrollToCursor()
			}
			p.updateViewport()
			return p, nil
		case "enter":
			if p.cursor < len(p.flat) {
				entry := p.flat[p.cursor]
				p.activeProvider = entry.provider
				p.activeModel = entry.model
				p.updateViewport()
				provider := p.providers[entry.provider]
				return p, func() tea.Msg {
					return messages.ModelSelectedMsg{
						Provider:     provider,
						ProviderName: entry.provider,
						Model:        entry.model,
					}
				}
			}
			return p, nil
		}
	}

	return p, nil
}

func (p *Pane) View() string {
	var header string
	if p.filtering {
		header = "  " + p.filter.View()
	} else if p.filter.Value() != "" {
		header = "  " + hintStyle.Render(fmt.Sprintf("filter: %s", p.filter.Value()))
	}
	if p.loading > 0 {
		if header != "" {
			header += "\n"
		}
		header += "  " + p.spinner.View() + " discovering models..."
	}

	help := hintStyle.Render("↑/↓ navigate  enter select  r refresh  / filter  esc clear")
	if !p.lastRefresh.IsZero() {
		help += hintStyle.Render(fmt.Sprintf("  last refresh: %s", p.lastRefresh.Format("15:04:05")))
	}

	var parts []string
	if header != "" {
		parts = append(parts, header)
	}
	parts = append(parts, p.viewport.View())
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (p *Pane) updateViewport() {
	var sb strings.Builder

	providerNames := p.sortedProviders()
	idx := 0
	for _, provName := range providerNames {
		sb.WriteString("  " + headerStyle.Render(provName))
		if err, ok := p.errors[provName]; ok {
			sb.WriteString("  " + errStyle.Render(fmt.Sprintf("(%v)", err)))
		}
		sb.WriteString("\n")

		hasModels := false
		for _, entry := range p.flat {
			if entry.provider != provName {
				continue
			}
			hasModels = true
			label := entry.model
			isActive := entry.provider == p.activeProvider && entry.model == p.activeModel

			if idx == p.cursor {
				sb.WriteString("  " + selectedStyle.Render(label))
			} else {
				sb.WriteString("  " + normalStyle.Render(label))
			}
			if isActive {
				sb.WriteString(activeMarker)
			}
			sb.WriteString("\n")
			idx++
		}
		if !hasModels {
			if _, ok := p.errors[provName]; !ok && p.loading == 0 {
				sb.WriteString("    " + hintStyle.Render("no models found") + "\n")
			}
		}
		sb.WriteString("\n")
	}

	p.viewport.SetContent(sb.String())
}

func (p *Pane) scrollToCursor() {
	// Each model is 1 line; count header lines before cursor to get the
	// actual line number in the viewport content.
	line := 0
	providerNames := p.sortedProviders()
	idx := 0
	for _, provName := range providerNames {
		line++ // provider header
		for _, entry := range p.flat {
			if entry.provider != provName {
				continue
			}
			if idx == p.cursor {
				goto found
			}
			idx++
			line++
		}
		line++ // blank line between groups
	}
found:
	vpHeight := p.viewport.Height
	if vpHeight < 1 {
		return
	}
	if line < p.viewport.YOffset {
		p.viewport.SetYOffset(line)
	} else if line >= p.viewport.YOffset+vpHeight {
		p.viewport.SetYOffset(line - vpHeight + 1)
	}
}

func (p *Pane) discoverAll() []tea.Cmd {
	var cmds []tea.Cmd
	p.loading = len(p.providers)
	riglog.Info("models: starting discovery for %d providers", len(p.providers))
	for name, prov := range p.providers {
		n, pr := name, prov
		cmds = append(cmds, func() tea.Msg {
			riglog.Info("models: discovering %s...", n)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			ms, err := pr.ListModels(ctx)
			return modelsLoadedMsg{provider: n, models: ms, err: err}
		})
	}
	cmds = append(cmds, p.spinner.Tick)
	return cmds
}

func (p *Pane) rebuildFlat() {
	p.flat = nil
	filterVal := strings.ToLower(p.filter.Value())
	for _, provName := range p.sortedProviders() {
		ms, ok := p.models[provName]
		if !ok {
			continue
		}
		for _, m := range ms {
			if filterVal != "" && !strings.Contains(strings.ToLower(m), filterVal) {
				continue
			}
			p.flat = append(p.flat, modelEntry{provider: provName, model: m})
		}
	}
	if p.cursor >= len(p.flat) {
		p.cursor = max(0, len(p.flat)-1)
	}
}

func (p *Pane) sortedProviders() []string {
	seen := make(map[string]bool)
	var names []string
	for name := range p.providers {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	for name := range p.errors {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func (p *Pane) persistModels() {
	if p.cfg == nil {
		return
	}
	if p.cfg.DiscoveredModels == nil {
		p.cfg.DiscoveredModels = make(map[string][]string)
	}
	for name, ms := range p.models {
		p.cfg.DiscoveredModels[name] = ms
	}
	p.cfg.Save()
}
