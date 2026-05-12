package messages

import "github.com/vicontiveros00/rig/internal/llm"

type ModelSelectedMsg struct {
	Provider    llm.Provider
	ProviderName string
	Model       string
}
