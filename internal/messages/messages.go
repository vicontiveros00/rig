package messages

import "github.com/vicontiveros00/rig/internal/llm"

type ModelSelectedMsg struct {
	Provider     llm.Provider
	ProviderName string
	Model        string
}

type ServersChangedMsg struct {
	Providers  map[string]llm.Provider
	MCPChanged bool
}

type ActivePlanChangedMsg struct {
	PlanTitle string
	PlanTasks string // markdown-formatted task list
}
