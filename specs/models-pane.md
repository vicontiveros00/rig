# models pane — auto-discovery and model switching

## overview

replace the models pane stub with an interactive view that auto-discovers
available models from all configured providers, displays them in a
filterable list, and lets the user switch the active model on the fly.
discovered models are cached to `~/.rig/config.yaml` so they persist
across sessions.

## goals

- zero manual model entry — just configure a provider endpoint and rig
  finds what's available
- fast model switching without restarting rig
- works with openai, ollama, litellm, and any openai-compatible proxy
- graceful handling of unreachable providers (show error, don't block)

## existing plumbing

the `Provider` interface already has `ListModels(ctx) ([]string, error)`:

- `internal/llm/provider.go` — interface definition
- `internal/llm/openai.go` — calls the openai `/v1/models` endpoint
- `internal/llm/ollama.go` — inherits from openai (ollama exposes
  the same endpoint at `localhost:11434/v1/models`)

the config struct (`internal/config/config.go`) currently has:

```go
type Config struct {
    DefaultProvider string
    DefaultModel    string
    Providers       map[string]ProviderConfig
}
```

## design

### config changes

add a `discovered_models` field to the config:

```yaml
discovered_models:
  openai:
    - gpt-4o
    - gpt-4o-mini
    - gpt-4-turbo
  ollama:
    - llama3
    - codellama
  litellm:
    - claude-sonnet-4-20250514
    - gpt-4o
```

```go
type Config struct {
    DefaultProvider  string                       `mapstructure:"default_provider"`
    DefaultModel     string                       `mapstructure:"default_model"`
    Providers        map[string]ProviderConfig    `mapstructure:"providers"`
    DiscoveredModels map[string][]string          `mapstructure:"discovered_models"`
}
```

add a `Save()` method to `config.go` that writes the current config
back to `~/.rig/config.yaml`.

### models pane ui

file: `internal/pane/models/models.go`

layout:
```
┌─────────────────────────────────────────────┐
│  filter: [__________]           r = refresh │
│                                             │
│  openai                                     │
│    gpt-4o                            ← active│
│    gpt-4o-mini                              │
│    gpt-4-turbo                              │
│                                             │
│  ollama                                     │
│    llama3                                   │
│    codellama                                │
│                                             │
│  litellm (error: connection refused)        │
│                                             │
│                          enter = select     │
└─────────────────────────────────────────────┘
```

- models are grouped by provider, each group has a header
- the currently active model is marked
- a text input at the top filters the list in real time
- `r` triggers a refresh (re-calls `ListModels` on all providers)
- `enter` on a model sets it as the active model + provider
- providers that fail to respond show an inline error under their header
- on first load, show cached models from config immediately, then
  refresh in the background

### discovery flow

1. on pane init (or `r` press), fire a `tea.Cmd` per provider that
   calls `ListModels()` concurrently
2. each result comes back as a `modelsLoadedMsg{provider, models, err}`
3. the pane merges results into its state, replacing the previous list
   for that provider
4. after all providers respond, persist to config via `Save()`

### model switching

when the user presses `enter` on a model:

1. emit a custom `ModelSelectedMsg{Provider, Model}` tea message
2. the app root intercepts this in its `Update` and:
   - updates its own `provider` and `model` fields (shown in status bar)
   - passes the message down to the chat pane
3. the chat pane receives `ModelSelectedMsg` and swaps its provider
   and model fields — the next message sent will use the new model
4. persist the new default to config

this requires:
- a shared message type (e.g. in `internal/pane/messages.go` or a
  small `internal/messages` package)
- the app root to intercept and broadcast it
- the chat pane to handle it

### keybindings

| key     | action                                |
|---------|---------------------------------------|
| `↑/↓`   | move cursor through model list       |
| `enter` | select model as active                |
| `r`     | refresh — re-discover from providers  |
| `/`     | focus the filter input                |
| `esc`   | clear filter / unfocus filter         |

## implementation steps

1. **config: add `DiscoveredModels` + `Save()`**
   - add field to struct
   - implement `Save()` that marshals config back to yaml
   - update default config template

2. **shared message type**
   - create `internal/messages/messages.go` with `ModelSelectedMsg`

3. **models pane: list + discovery**
   - replace stub with real pane struct
   - state: `models map[string][]string`, `errors map[string]error`,
     `cursor int`, `filter string`, `loading bool`
   - `Init()` returns cmd to discover from all providers
   - `Update()` handles key navigation, filter input, selection
   - `View()` renders grouped list with active marker

4. **models pane: refresh + persistence**
   - `r` key fires concurrent `ListModels` cmds
   - on completion, call `config.Save()` to persist

5. **model switching: app root + chat pane**
   - app root intercepts `ModelSelectedMsg`, updates status bar,
     creates new provider if needed, forwards to chat pane
   - chat pane swaps its `provider` and `model` fields

6. **polish**
   - show spinner during discovery
   - handle empty model lists gracefully
   - show timestamp of last refresh in status area

## files touched

- `internal/config/config.go` — add `DiscoveredModels`, `Save()`
- `internal/messages/messages.go` — new, shared message types
- `internal/pane/models/models.go` — full rewrite from stub
- `internal/app/app.go` — intercept `ModelSelectedMsg`
- `internal/pane/chat/chat.go` — handle `ModelSelectedMsg`
- `cmd/rig/main.go` — pass all providers to models pane constructor
