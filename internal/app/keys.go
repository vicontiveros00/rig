package app

import (
	"github.com/charmbracelet/bubbles/key"
)

type keyMap struct {
	NextTab key.Binding
	PrevTab key.Binding
	Quit    key.Binding
}

var keys = keyMap{
	NextTab: key.NewBinding(
		key.WithKeys("ctrl+right", "ctrl+l"),
		key.WithHelp("ctrl+→", "next tab"),
	),
	PrevTab: key.NewBinding(
		key.WithKeys("ctrl+left", "ctrl+h"),
		key.WithHelp("ctrl+←", "prev tab"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl+c", "quit"),
	),
}
