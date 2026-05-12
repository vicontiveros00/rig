package servers

import "github.com/vicontiveros00/rig/internal/pane"

func New() pane.Pane {
	return pane.NewStub("Servers", "launch and manage servers")
}
