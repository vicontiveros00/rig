# chat widget — unified reusable chat component

## overview

extract the chat UI (textarea input, viewport, spinner, glamour
rendering, streaming state) into a single reusable widget in
`internal/chatcore`. each pane that needs chat functionality embeds
the widget and only provides two things:

1. a system prompt builder function
2. an optional post-stream callback (for tool block detection, auto-save, etc.)

the chat pane, plan pane chat mode, and build pane agent mode all
use the same widget. the visual presentation is identical everywhere.

## problem

the same chat UI code is duplicated three times:

- **chat pane**: textarea, viewport, spinner, glamour, streaming,
  `updateViewportContent`, `View` with status/help
- **plan pane**: `chatInput`, `chatVP`, `chatSpinner`, glamour renderer,
  `updateChatViewport`, `viewChat`
- **build pane**: `agentInput`, `agentVP`, `agentSpin`,
  `updateAgentViewport`, `viewAgent`

each copy has the same bugs fixed independently (blink restart,
spinner always ticking, viewport scrolling, glamour rendering).

## design

### ChatWidget struct

```go
// in internal/chatcore/widget.go

type ChatWidget struct {
    Engine   Engine
    Input    textarea.Model
    Viewport viewport.Model
    Spinner  spinner.Model
    Renderer *glamour.TermRenderer
    Err      error

    width  int
    height int
}
```

### constructor

```go
func NewWidget(provider llm.Provider, model string) *ChatWidget
```

creates the textarea, spinner, glamour renderer with sensible defaults.

### methods

| method | purpose |
|--------|---------|
| `SetSize(w, h int)` | resize viewport, textarea, recreate glamour renderer |
| `Init() tea.Cmd` | returns `textarea.Blink` + `spinner.Tick` |
| `Send(text, systemPrompt string) tea.Cmd` | clears input, appends user+assistant, starts stream |
| `HandleKey(msg tea.KeyMsg) (tea.Cmd, bool)` | handles esc-to-cancel during stream, forwards to textarea+viewport. returns `(cmd, handled)` |
| `HandleChunk(msg ChunkMsg) (done bool, cmd tea.Cmd)` | processes chunk, updates viewport, returns next chunk cmd or nil |
| `HandleTick(msg spinner.TickMsg) tea.Cmd` | updates spinner |
| `View(status string) string` | renders viewport + status + spinner + textarea + help |
| `UpdateViewport(contentTransform func(string) string)` | rebuilds viewport content with optional transform (for stripping tool blocks) |
| `Refocus() tea.Cmd` | focuses textarea, returns Blink cmd |
| `Clear()` | clears messages |
| `SetProvider(p llm.Provider, model string)` | delegate to engine |

### content transform

the `UpdateViewport` method accepts an optional transform function
that post-processes assistant content before rendering. this is how
each pane customizes rendering:

- **chat pane**: no transform (or nil) — plain glamour rendering
- **plan pane**: `StripPlanToolBlocks` — removes `<tool:...>` tags
- **build pane**: `stripToolBlocks` — removes `<tool:...>` tags

### how each pane uses it

**chat pane** — simplest case:
```go
type Pane struct {
    chat *chatcore.ChatWidget
    // ... session management, picker, plan context
}
```
- on `enter`: `p.chat.Send(text, p.buildSystemPrompt())`
- on `ChunkMsg`: `done, cmd := p.chat.HandleChunk(msg)` → if done, auto-save
- `View()`: `p.chat.View("ctrl+n new  ctrl+o history")`

**plan pane** — embedded in chat mode:
```go
type Pane struct {
    chat *chatcore.ChatWidget
    // ... task list, plan state, pending tool
}
```
- on `c`: switch to chat view, `p.chat.Refocus()`
- on `enter`: `p.chat.Send(text, p.planSystemPrompt())`
- on `ChunkMsg`: `done, cmd := p.chat.HandleChunk(msg)` → if done, check tool blocks
- `viewChat()`: `p.chat.View("y=apply n=skip")` or `p.chat.View("enter=send esc=back")`
- viewport transform: `StripPlanToolBlocks`

**build pane** — embedded in agent mode:
```go
type Pane struct {
    agent *chatcore.ChatWidget
    // ... command runner, pending tool, history
}
```
- same pattern, different system prompt and tool vocabulary

## what this eliminates

per pane, ~80-100 lines of:
- textarea/viewport/spinner field declarations
- `SetSize` logic for textarea + viewport + glamour
- `Init` returning blink + tick
- viewport content building loop
- glamour rendering of assistant messages
- spinner tick handling
- streaming state checks in view
- textarea blink restart after stream

all of this moves into the widget once.

## implementation steps

1. create `internal/chatcore/widget.go` with `ChatWidget`
2. refactor chat pane to use `ChatWidget`
3. refactor plan pane chat mode to use `ChatWidget`
4. refactor build pane agent mode to use `ChatWidget`
5. remove duplicate code from all three panes
6. build and verify

## files touched

- `internal/chatcore/widget.go` — new, the unified chat widget
- `internal/pane/chat/chat.go` — simplify to use widget
- `internal/pane/plan/plan.go` — simplify chat mode to use widget
- `internal/pane/build/build.go` — simplify agent mode to use widget
