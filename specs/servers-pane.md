# servers pane — manage model providers and mcp servers

## overview

replace the servers pane stub with a unified view for managing all
server connections: cloud model providers (openai, anthropic, etc.),
local model servers (ollama, omlx, litellm), and mcp servers (e.g.
discord bot). users can add, edit, remove, and test connectivity for
each server directly from the tui. api keys can be left empty for
local servers that don't require authentication.

relates to: https://github.com/vicontiveros00/rig/issues/6

## goals

- single pane to manage all server types (model providers + mcp)
- add new servers without editing config files by hand
- test server reachability with a health check from the ui
- support empty api keys for local/unauthenticated servers
- persist server config to `~/.rig/config.yaml`
- start/stop local servers (ollama, omlx) from the ui when possible

## server types

### model providers

these expose an openai-compatible `/v1/chat/completions` endpoint (and
optionally `/v1/models` for discovery). includes cloud providers like
openai and anthropic, as well as local servers like ollama and litellm.

fields:
- **name** — user-defined label (e.g. "openai", "local-ollama", "litellm-proxy")
- **endpoint** — base url (e.g. `https://api.openai.com/v1`, `http://localhost:11434/v1`)
- **api_key** — authentication key; empty string if not required
- **type** — `cloud` | `local` (affects ui grouping and start/stop controls)

### mcp servers

these are model context protocol servers that provide tool/resource
access to the llm (e.g. a discord bot exposing channels as tools).

fields:
- **name** — user-defined label (e.g. "discord-mcp", "filesystem-mcp")
- **endpoint** — server address (e.g. `http://localhost:8080`, `stdio://path/to/binary`)
- **api_key** — authentication key; empty string if not required
- **transport** — `sse` | `stdio` (how rig communicates with the mcp server)
- **autostart** — bool; whether to launch the server when rig starts

## config changes

extend the config struct and yaml schema:

```yaml
providers:
  openai:
    endpoint: https://api.openai.com/v1
    api_key: ""
    type: cloud

  ollama:
    endpoint: http://localhost:11434/v1
    api_key: ""
    type: local

mcp_servers:
  discord-mcp:
    endpoint: http://localhost:8080
    api_key: ""
    transport: sse
    autostart: false

  filesystem-mcp:
    endpoint: stdio:///usr/local/bin/fs-mcp
    api_key: ""
    transport: stdio
    autostart: true
```

```go
type ProviderConfig struct {
    Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`
    APIKey   string `mapstructure:"api_key" yaml:"api_key"`
    Type     string `mapstructure:"type" yaml:"type"` // "cloud" or "local"
}

type MCPServerConfig struct {
    Endpoint  string `mapstructure:"endpoint" yaml:"endpoint"`
    APIKey    string `mapstructure:"api_key" yaml:"api_key"`
    Transport string `mapstructure:"transport" yaml:"transport"` // "sse" or "stdio"
    Autostart bool   `mapstructure:"autostart" yaml:"autostart"`
}

type Config struct {
    DefaultProvider  string                       `mapstructure:"default_provider" yaml:"default_provider"`
    DefaultModel     string                       `mapstructure:"default_model" yaml:"default_model"`
    Providers        map[string]ProviderConfig    `mapstructure:"providers" yaml:"providers"`
    MCPServers       map[string]MCPServerConfig   `mapstructure:"mcp_servers" yaml:"mcp_servers,omitempty"`
    DiscoveredModels map[string][]string          `mapstructure:"discovered_models" yaml:"discovered_models,omitempty"`
    path             string
}
```

add a `SaveConfig()` method (distinct from the existing `Save()` which
handles the models cache) that writes provider and mcp server changes
back to `~/.rig/config.yaml`.

## servers pane ui

file: `internal/pane/servers/servers.go`

layout:
```
┌─────────────────────────────────────────────────────────────┐
│  ── model providers ──────────────────────────────────────  │
│                                                             │
│    openai           https://api.openai.com/v1    ● online   │
│    ollama           http://localhost:11434/v1     ○ offline  │
│    anthropic        https://api.anthropic.com    ● online   │
│                                                             │
│  ── mcp servers ──────────────────────────────────────────  │
│                                                             │
│    discord-mcp      http://localhost:8080         ● online   │
│    filesystem-mcp   stdio:///usr/local/bin/fs…   ○ stopped  │
│                                                             │
│  a = add  e = edit  d = delete  t = test  s = start/stop   │
└─────────────────────────────────────────────────────────────┘
```

- servers are grouped into two sections: model providers and mcp servers
- each row shows name, endpoint (truncated if needed), and status indicator
- status: `● online` (green), `○ offline` (red), `○ stopped` (gray)
- cursor highlights the currently selected row
- footer shows available keybindings

### add/edit form

when adding or editing a server, an inline form appears:

```
┌─────────────────────────────────────────────────────────────┐
│  add model provider                                         │
│                                                             │
│  name:      [__________________]                            │
│  endpoint:  [__________________]                            │
│  api key:   [__________________]  (leave empty if none)     │
│  type:      [cloud ▼]                                       │
│                                                             │
│                    enter = save    esc = cancel              │
└─────────────────────────────────────────────────────────────┘
```

for mcp servers the form includes transport and autostart fields instead of type.

## health check

`t` on a selected server fires a connectivity test:

- **model providers**: HTTP GET to `{endpoint}/models` — success if 2xx
  response (even if api_key is wrong, the connection itself is valid;
  a 401 means "reachable but unauthorized" which is still useful info)
- **mcp servers (sse)**: HTTP GET to the endpoint root — success if 2xx
- **mcp servers (stdio)**: attempt to spawn the binary and read the
  initialization handshake

results display inline as status text (replaces the status indicator
temporarily with the test result).

## start/stop local servers

for servers marked `type: local` or mcp servers with `autostart`:

- `s` on a local model provider (e.g. ollama) runs the server binary
  (`ollama serve`) in the background and monitors the pid
- `s` on an mcp stdio server spawns the binary
- stopping sends SIGTERM and waits up to 5s before SIGKILL
- status updates reactively in the ui

this requires a simple process manager embedded in the pane:

```go
type managedProcess struct {
    name    string
    cmd     *exec.Cmd
    running bool
}
```

## keybindings

| key     | action                                           |
|---------|--------------------------------------------------|
| `↑/↓`  | move cursor through server list                  |
| `a`    | open add form (prompts for provider or mcp type) |
| `e`    | edit selected server                             |
| `d`    | delete selected server (with confirmation)       |
| `t`    | test connectivity / health check                 |
| `s`    | start/stop local server                          |
| `enter`| expand details of selected server                |
| `esc`  | close form / collapse details                    |

## implementation steps

1. **config: add `MCPServerConfig` + `Type` field + `SaveConfig()`**
   - add `MCPServerConfig` struct
   - add `Type` field to `ProviderConfig`
   - add `MCPServers` map to `Config`
   - implement `SaveConfig()` that writes providers and mcp_servers
     back to yaml (preserving comments where possible)
   - update default config template with type annotations

2. **servers pane: list view**
   - replace stub with real pane struct
   - state: providers list, mcp servers list, cursor position,
     selected section, status map
   - `Init()` returns cmd to health-check all servers
   - `View()` renders grouped list with status indicators

3. **servers pane: add/edit form**
   - inline form using `textinput` bubbles
   - tab between fields, enter to save, esc to cancel
   - on save: validate endpoint format, persist via `SaveConfig()`
   - emit message so models pane can refresh its provider list

4. **servers pane: health check**
   - implement `checkHealth(endpoint, apiKey string) error` for http
   - implement stdio health check (spawn + read init)
   - results come back as `healthResultMsg{name, status, err}`
   - `t` triggers check for selected; on init, check all concurrently

5. **servers pane: start/stop**
   - process manager for local servers
   - `s` toggles start/stop
   - monitor process state, update status reactively
   - cleanup on app exit (kill managed processes)

6. **servers pane: delete with confirmation**
   - `d` shows "delete X? y/n" prompt inline
   - on confirm: remove from config, persist, remove from ui list

7. **integration: wire into main + app**
   - pass `*config.Config` to servers pane constructor
   - emit `ServersChangedMsg` when providers are added/removed so
     the models pane and chat pane can react
   - app root forwards `ServersChangedMsg` to all panes

8. **mcp pane consolidation** (optional follow-up)
   - consider whether the existing `mcp` pane stub should be merged
     into the servers pane or remain separate for mcp tool browsing

## files touched

- `internal/config/config.go` — add `MCPServerConfig`, `Type`, `MCPServers`, `SaveConfig()`
- `internal/pane/servers/servers.go` — full rewrite from stub
- `internal/messages/messages.go` — add `ServersChangedMsg`
- `internal/app/app.go` — handle `ServersChangedMsg`
- `cmd/rig/main.go` — pass config to servers pane constructor
- `internal/pane/models/models.go` — handle `ServersChangedMsg` to refresh providers

## open questions

- should mcp server management live solely in the servers pane, or
  should the separate `mcp` pane remain for tool/resource browsing?
- for stdio mcp servers, should rig manage the process lifecycle or
  just record the endpoint for an externally-managed server?
- should api keys be masked in the ui (show `••••••` instead of plaintext)?
