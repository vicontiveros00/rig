package git

import "github.com/vicontiveros00/rig/internal/pane"

func New() pane.Pane {
	return pane.NewStub("Git", "status, diff, commit, push")
}
