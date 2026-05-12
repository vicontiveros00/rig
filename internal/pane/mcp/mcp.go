package mcp

import "github.com/vicontiveros00/rig/internal/pane"

func New() pane.Pane {
	return pane.NewStub("mcp", "manage mcp servers")
}
