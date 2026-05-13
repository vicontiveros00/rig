# mcp pane — browse and invoke mcp tools and resources

## overview

replace the mcp pane stub with an interactive view that connects to
configured mcp servers, lists their available tools and resources,
and lets the user invoke tools or read resources directly from the ui.
this pane is the "tool browser" — server lifecycle management lives in
the servers pane.

relates to: https://github.com/vicontiveros00/rig/issues/5

## goals

- connect to all configured mcp servers on pane init
- list available tools and resources grouped by server
- invoke any tool with json arguments from an inline input
- display tool results and resource contents in-pane
- handle connection failures gracefully (show error per server)
- react to `ServersChangedMsg` to refresh when mcp servers are
  added/removed from the servers pane

## mcp protocol basics

mcp uses a json-rpc 2.0 transport (over sse or stdio) with these
relevant methods:

- `initialize` — handshake, exchange capabilities
- `tools/list` — returns available tools with names, descriptions, schemas
- `resources/list` — returns available resources with uris and descriptions
- `tools/call` — invoke a tool with arguments, returns result
- `resources/read` — read a resource by uri, returns content

## design

### mcp client

file: `internal/mcp/client.go`

a lightweight client that speaks json-rpc over sse (http) for now.
stdio support can be added later.

```go
type Client struct {
    endpoint  string
    apiKey    string
    transport string
}

type Tool struct {
    Name        string
    Description string
    InputSchema map[string]any
}

type Resource struct {
    URI         string
    Name        string
    Description string
    MimeType    string
}

type ToolResult struct {
    Content string
    IsError bool
}
```

methods:
- `Connect(ctx) error` — performs initialize handshake
- `ListTools(ctx) ([]Tool, error)`
- `ListResources(ctx) ([]Resource, error)`
- `CallTool(ctx, name string, args map[string]any) (ToolResult, error)`
- `ReadResource(ctx, uri string) (string, error)`

### mcp pane ui

file: `internal/pane/mcp/mcp.go`

layout (tool list mode):
```
┌─────────────────────────────────────────────────────────────┐
│  ── discord-mcp (connected) ────────────────────────────── │
│                                                             │
│  tools:                                                     │
│    send_message     Send a message to a channel             │
│    list_channels    List available channels                  │
│    get_history      Get message history                      │
│                                                             │
│  resources:                                                 │
│    discord://guilds           Available guilds               │
│    discord://channels/123     Channel details                │
│                                                             │
│  ── filesystem-mcp (error: not running) ──────────────────  │
│                                                             │
│  r = refresh  enter = invoke/read  / = filter               │
└─────────────────────────────────────────────────────────────┘
```

layout (tool invoke mode):
```
┌─────────────────────────────────────────────────────────────┐
│  invoke: send_message                                       │
│  Send a message to a channel                                │
│                                                             │
│  args (json):                                               │
│  [{"channel": "general", "content": "hello"}___________]    │
│                                                             │
│                    enter = execute    esc = cancel           │
│─────────────────────────────────────────────────────────────│
│  result:                                                    │
│  {"status": "sent", "message_id": "123456"}                 │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

### state

```go
type MCP struct {
    cfg      *config.Config
    clients  map[string]*mcp.Client
    tools    map[string][]mcp.Tool      // server -> tools
    resources map[string][]mcp.Resource // server -> resources
    errors   map[string]error

    entries  []entry  // flattened list for cursor navigation
    cursor   int
    filter   string

    invokeMode bool
    invokeTarget string
    argsInput  textinput.Model
    result     string
    resultErr  bool
}
```

### connection flow

1. on `Init()`, connect to each mcp server in `cfg.MCPServers`
   concurrently (only those with transport "sse" for now)
2. on successful connect, call `ListTools` and `ListResources`
3. results arrive as messages, pane updates its state
4. on `ServersChangedMsg`, reconnect to any new servers

### tool invocation

1. user selects a tool and presses `enter`
2. pane switches to invoke mode showing tool description and a
   json args input field
3. user types json args and presses `enter`
4. pane fires `CallTool` and displays the result below
5. `esc` returns to list mode

### resource reading

1. user selects a resource and presses `enter`
2. pane calls `ReadResource` and displays content inline
3. `esc` returns to list mode

## keybindings

| key     | action                                        |
|---------|-----------------------------------------------|
| `↑/↓`  | move cursor through tool/resource list        |
| `enter`| invoke selected tool / read selected resource |
| `r`    | refresh — reconnect and re-list all servers   |
| `/`    | focus filter input                            |
| `esc`  | cancel invoke / clear filter / back to list   |

## implementation steps

1. **mcp client package**
   - create `internal/mcp/client.go` with SSE json-rpc transport
   - implement `Connect`, `ListTools`, `ListResources`, `CallTool`,
     `ReadResource`
   - keep it simple: HTTP POST for requests, SSE stream for events

2. **mcp pane: list view**
   - replace stub with real pane struct
   - on init, connect to all configured mcp servers concurrently
   - display tools and resources grouped by server
   - show connection errors inline

3. **mcp pane: invoke mode**
   - `enter` on a tool opens invoke mode
   - json args input using `textinput` bubble
   - execute and display result

4. **mcp pane: resource reading**
   - `enter` on a resource calls `ReadResource`
   - display content in a scrollable view

5. **integration**
   - pass `*config.Config` to mcp pane constructor
   - handle `ServersChangedMsg` to refresh client list
   - update `main.go` to pass config

## files touched

- `internal/mcp/client.go` — new, mcp json-rpc client
- `internal/pane/mcp/mcp.go` — full rewrite from stub
- `cmd/rig/main.go` — pass config to mcp pane constructor
