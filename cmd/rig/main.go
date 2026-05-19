package main

import (
	// earlyinit must load before bubbletea to prevent terminal color queries
	_ "github.com/vicontiveros00/rig/internal/earlyinit"

	"flag"
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/vicontiveros00/rig/internal/app"
	"github.com/vicontiveros00/rig/internal/config"
	"github.com/vicontiveros00/rig/internal/llm"
	riglog "github.com/vicontiveros00/rig/internal/log"
	"github.com/vicontiveros00/rig/internal/pane"
	"github.com/vicontiveros00/rig/internal/pane/build"
	"github.com/vicontiveros00/rig/internal/pane/chat"
	"github.com/vicontiveros00/rig/internal/pane/git"
	"github.com/vicontiveros00/rig/internal/pane/mcp"
	"github.com/vicontiveros00/rig/internal/pane/models"
	"github.com/vicontiveros00/rig/internal/pane/plan"
	"github.com/vicontiveros00/rig/internal/pane/scratch"
	"github.com/vicontiveros00/rig/internal/pane/servers"
	"github.com/vicontiveros00/rig/internal/project"
)

func main() {
	providerFlag := flag.String("provider", "", "LLM provider to use (overrides config)")
	modelFlag := flag.String("model", "", "Model to use (overrides config)")
	flag.Parse()

	riglog.Init()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}
	riglog.Info("config loaded: provider=%s model=%s providers=%d", cfg.DefaultProvider, cfg.DefaultModel, len(cfg.Providers))

	providerName := cfg.DefaultProvider
	modelName := cfg.DefaultModel
	if *providerFlag != "" {
		providerName = *providerFlag
	}
	if *modelFlag != "" {
		modelName = *modelFlag
	}

	allProviders := make(map[string]llm.Provider)
	for name, pcfg := range cfg.Providers {
		p, err := llm.NewProvider(name, pcfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not create provider %q: %v\n", name, err)
			continue
		}
		allProviders[name] = p
	}

	activeProvider, ok := allProviders[providerName]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown provider %q — available: ", providerName)
		for k := range allProviders {
			fmt.Fprintf(os.Stderr, "%s ", k)
		}
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	panes := []pane.Pane{
		chat.New(activeProvider, modelName),
		scratch.New(),
		plan.New(activeProvider, modelName),
		build.New(activeProvider, modelName),
		git.New(),
		mcp.New(cfg),
		models.New(allProviders, cfg, providerName, modelName),
		servers.New(cfg, allProviders),
	}

	projectName := ""
	if root, ok := project.DetectRoot(); ok {
		projectName = filepath.Base(root)
	}

	m := app.New(panes, providerName, modelName, projectName)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
