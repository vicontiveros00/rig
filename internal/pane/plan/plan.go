package plan

import "github.com/vicontiveros00/rig/internal/pane"

func New() pane.Pane {
	return pane.NewStub("Plan", "structured task planning")
}
