package models

import "github.com/vicontiveros00/rig/internal/pane"

func New() pane.Pane {
	return pane.NewStub("Models", "local and cloud models")
}
