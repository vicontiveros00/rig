# rig

Your final AI interface. A multi-pane terminal UI for managing LLMs, MCP servers, git, builds, and more — all from one command.

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

## Usage

```bash
rig                          # launch with defaults from config
rig --provider ollama        # override provider
rig --model gpt-4o-mini      # override model
```

### Navigation

| Key | Action |
|---|---|
| `Ctrl+→` / `Ctrl+L` | Next tab |
| `Ctrl+←` / `Ctrl+H` | Previous tab |
| `Enter` | Send message (in Chat) |
| `Alt+Enter` | Newline in input |
| `Esc` | Cancel streaming |
| `Ctrl+S` | Save (in Scratch) |
| `Ctrl+C` | Quit |

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

## Architecture

rig is built with the [Charm](https://charm.sh) stack for Go:

| Layer | Library | Role |
|---|---|---|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) | Elm-architecture event loop |
| Styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) | Terminal CSS |
| Components | [Bubbles](https://github.com/charmbracelet/bubbles) | Text input, viewport, spinner |
| Markdown | [Glamour](https://github.com/charmbracelet/glamour) | Render markdown in terminal |
| Config | [Viper](https://github.com/spf13/viper) | YAML config with env overrides |

The root `App` model owns all pane models and delegates `Update`/`View` to the active pane. Each pane implements a common `Pane` interface so the app treats them uniformly. LLM providers implement a `Provider` interface with streaming support, making it straightforward to add new backends.

### Project structure

```
rig/
├── cmd/rig/main.go                 # entry point, flags, wiring
├── internal/
│   ├── app/                        # root Bubble Tea model, pane routing, keybindings
│   ├── pane/                       # Pane interface + all pane implementations
│   │   ├── chat/                   # LLM chat with streaming + markdown rendering
│   │   ├── scratch/                # persistent notepad (saves to ~/.rig/scratch.md)
│   │   ├── plan/                   # structured task planning (stub)
│   │   ├── build/                  # run builds and commands (stub)
│   │   ├── git/                    # git operations (stub)
│   │   ├── mcp/                    # MCP server management (stub)
│   │   ├── models/                 # model browser (stub)
│   │   └── servers/                # server management (stub)
│   ├── llm/                        # Provider interface, OpenAI + Ollama backends
│   ├── config/                     # Viper-based config (~/.rig/config.yaml)
│   └── ui/                         # shared styles, tab bar, status bar
├── go.mod
├── Makefile                        # build, run, install, clean
└── README.md
```

## Panes

| Pane | Status | Description |
|---|---|---|
| **Chat** | Working | Send prompts and stream responses from any LLM. Markdown rendering, cancel with Esc. |
| **Scratch** | Working | Persistent notepad. Saves to `~/.rig/scratch.md` with `Ctrl+S`. Will support session tagging. |
| **Plan** | Stub | Structured task planning. |
| **Build** | Stub | Run builds and commands, see output. |
| **Git** | Stub | Status, diff, commit, push. |
| **MCP** | Stub | Connect to and manage MCP servers. |
| **Models** | Stub | Browse and switch between local and cloud models. |
| **Servers** | Stub | Launch, monitor, and kill local servers (LLM, MCP, etc.). |

## LLM Providers

rig supports any OpenAI-compatible API out of the box:

- **OpenAI** — GPT-4o, GPT-4o-mini, etc.
- **Ollama** — any local model via the OpenAI-compatible endpoint at `localhost:11434/v1`
- **Any OpenAI-compatible server** — set a custom endpoint in config

Streaming is handled via a `Provider` interface with a channel-based `StreamChat` method, so adding new providers (Anthropic, Groq, local vLLM, etc.) is a matter of implementing the interface.

## Development

```bash
make build          # compile to bin/rig
make run            # go run
make install        # go install to $GOPATH/bin
make clean          # remove bin/
```

## License

MIT
