# rig

Your final AI interface. A multi-pane terminal UI for chatting with LLMs, planning tasks, running agentic builds, managing git repos, browsing MCP tools, and switching models — all from one command.

## Install

```bash
go install github.com/vicontiveros00/rig/cmd/rig@latest
```

Or build from source:

```bash
git clone https://github.com/vicontiveros00/rig.git
cd rig
make install
```

Make sure `~/go/bin` is in your `$PATH`.

## Usage

```bash
rig                          # launch with defaults from config
rig --provider ollama        # override provider
rig --model gpt-4o-mini      # override model
```

Launch rig from any project directory — it auto-detects the project root and provides file tree context to Rigby.

## Panes

| Pane | Description |
|---|---|
| **Chat** | General-purpose conversation with Rigby. Markdown rendering, conversation history (ctrl+o), auto-save. |
| **Scratch** | Persistent notepad with history. Save (ctrl+s), new (ctrl+n), browse past scratches (ctrl+o). |
| **Plan** | Task planning with add/edit/delete/indent, status toggling. Built-in plan chat — Rigby proposes tasks via tool calls, you approve with y/n. Active plan provides context to chat and build panes. |
| **Build** | Manual command runner + agentic mode. Rigby runs commands (run_cmd), reads files (read_file), and iterates until done — each action requires your approval. Command history persists. |
| **Git** | Repo status, colored diffs, stage/unstage, commit, push, pull, fetch, stash, branch switching. Clone prompt when no repo detected. |
| **MCP** | Connect to MCP servers, browse tools and resources, invoke tools with JSON args, view results. Supports Streamable HTTP (SSE) transport. |
| **Models** | Auto-discover models from all providers, filter, switch the active model on the fly. |
| **Servers** | Add/edit/remove model providers and MCP servers, health checks, start/stop local servers. |

### Navigation

| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Switch panes |
| `Ctrl+C` | Quit |

### Chat / Plan chat / Build agent

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Alt+Enter` | Newline in input |
| `Esc` | Cancel streaming / back to list |
| `Ctrl+N` | New session |
| `Ctrl+O` | Browse history |

### Plan (list mode)

| Key | Action |
|---|---|
| `a` | Add task |
| `e` | Edit task title |
| `Space` | Cycle status (pending → in progress → done) |
| `d` | Delete task |
| `n` | Edit notes |
| `c` | Open plan chat with Rigby |
| `Tab` / `Shift+Tab` | Indent / unindent task |

### Build (manual mode)

| Key | Action |
|---|---|
| `Enter` | Run command |
| `Ctrl+C` | Kill running command |
| `Ctrl+L` | Clear output |
| `↑` / `↓` | Command history |
| `c` | Switch to agent mode |

### Git

| Key | Action |
|---|---|
| `Enter` | View diff |
| `a` / `u` | Stage / unstage file |
| `A` | Stage all |
| `c` | Commit (inline message) |
| `p` / `P` | Push / pull |
| `s` / `S` | Stash / stash pop |
| `f` | Fetch all |
| `b` | Browse branches |
| `l` | View log |

## Configuration

On first run, rig creates `~/.rig/config.yaml`:

```yaml
default_provider: openai
default_model: gpt-4o

providers:
  openai:
    endpoint: https://api.openai.com/v1
    api_key: ""

  ollama:
    endpoint: http://localhost:11434/v1
    api_key: ""

  anthropic:
    endpoint: https://api.anthropic.com
    api_key: ""
```

API keys can also be set via environment variables: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`.

Providers and MCP servers can be managed directly from the **servers** pane — no need to edit YAML by hand.

## Data storage

| Path | Contents |
|---|---|
| `~/.rig/config.yaml` | Provider and MCP server configuration |
| `~/.rig/history/chat/` | Saved chat conversations (JSON) |
| `~/.rig/history/scratch/` | Archived scratch notes (Markdown) |
| `~/.rig/history/plan/` | Saved plans with tasks (JSON) |
| `~/.rig/active_plan` | ID of the currently active plan |
| `~/.rig/build_history` | Recent build commands |
| `~/.rig/models_cache.yaml` | Discovered models cache |

## Architecture

Built with the [Charm](https://charm.sh) stack for Go:

| Layer | Library |
|---|---|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| Components | [Bubbles](https://github.com/charmbracelet/bubbles) |
| Markdown | [Glamour](https://github.com/charmbracelet/glamour) |
| Config | [Viper](https://github.com/spf13/viper) |
| LLM | [go-openai](https://github.com/sashabaranov/go-openai) |

### Project structure

```
rig/
├── cmd/rig/                    # entry point, flags, wiring
├── internal/
│   ├── app/                    # root model, pane routing, keybindings
│   ├── chatcore/               # shared chat engine (streaming, messages, token tracking)
│   ├── config/                 # Viper config + MCP server config
│   ├── history/                # persistence for chats, scratches, plans
│   ├── llm/                    # Provider interface, OpenAI + Ollama backends
│   ├── mcp/                    # MCP JSON-RPC client (SSE transport)
│   ├── project/                # project root detection, file tree, file reading
│   ├── pane/
│   │   ├── chat/               # general chat with Rigby
│   │   ├── scratch/            # persistent notepad with history
│   │   ├── plan/               # task planning + plan chat + tool-call apply
│   │   ├── build/              # command runner + agentic build agent
│   │   ├── git/                # git status, diff, commit, branches, stash
│   │   ├── mcp/                # MCP tool/resource browser
│   │   ├── models/             # model discovery and switching
│   │   └── servers/            # provider and MCP server management
│   ├── ui/                     # shared styles, tab bar, status bar
│   └── version/                # build-time version injection
├── specs/                      # design specs for each feature
├── go.mod
├── Makefile
└── README.md
```

## Development

```bash
make build          # compile to bin/rig
make run            # go run
make install        # go install to $GOPATH/bin
make clean          # remove bin/
```

## License

MIT
