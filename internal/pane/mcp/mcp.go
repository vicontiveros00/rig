package mcp

import "github.com/vicontiveros00/rig/internal/pane"

func New() pane.Pane {
	return pane.NewStub("MCP", "manage MCP servers")
}
