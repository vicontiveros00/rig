# build pane — run commands and view output

## overview

replace the build pane stub with an interactive shell runner. the user
types commands, sees real-time streaming output in a scrollable viewport,
and can run multiple commands in sequence. exit codes are displayed per
command. the active plan provides context for what the user is building.

relates to: https://github.com/vicontiveros00/rig/issues/2

## goals

- run arbitrary shell commands from within rig
- stream stdout/stderr in real time to a scrollable viewport
- show exit codes and duration for each command
- support a command history (up arrow recalls previous commands)
- display the active plan title for context
- kill running commands with `ctrl+c`
- multiple commands can be run in sequence (output accumulates)

## design

### layout

```
┌─────────────────────────────────────────────────────────────┐
│  build                                    plan: add mcp     │
│─────────────────────────────────────────────────────────────│
│  $ go build ./...                                           │
│  (exit 0, 2.1s)                                             │
│                                                             │
│  $ go test ./internal/config/...                            │
│  ok   github.com/vicontiveros00/rig/internal/config  0.42s  │
│  (exit 0, 0.5s)                                             │
│                                                             │
│  $ go vet ./...                                             │
│  # github.com/vicontiveros00/rig/internal/pane/chat         │
│  chat.go:42: unreachable code                               │
│  (exit 1, 1.3s)                                             │
│                                                             │
│─────────────────────────────────────────────────────────────│
│  > go build ./...                                           │
│  ctrl+c=kill  ctrl+l=clear  ↑=prev cmd                     │
└─────────────────────────────────────────────────────────────┘
```

### state

```go
type commandRun struct {
    cmd      string
    output   string
    exitCode int
    duration time.Duration
    running  bool
}

type Pane struct {
    runs     []commandRun
    input    textinput.Model
    viewport viewport.Model
    width    int
    height   int

    // Active process
    proc     *exec.Cmd
    cancel   context.CancelFunc
    running  bool

    // Command history
    history    []string
    historyIdx int

    // Plan context
    activePlanTitle string
    activePlanTasks string
}
```

### command execution

when the user presses `enter`:

1. append a new `commandRun` to `runs` with `running: true`
2. spawn the command via `exec.CommandContext` with a cancel context
3. pipe stdout+stderr into a combined reader
4. stream output via tea messages (`buildOutputMsg{line string}`)
5. on completion, record exit code and duration
6. update viewport

commands run in the user's shell (`$SHELL -c "command"`) so aliases,
PATH, and environment are inherited.

### streaming output

a background goroutine reads from the command's combined output pipe
and sends lines back via tea messages:

```go
type buildOutputMsg struct {
    line string
}

type buildDoneMsg struct {
    exitCode int
    duration time.Duration
}
```

the viewport content is rebuilt after each `buildOutputMsg` — new lines
append to the current run's output, and the viewport scrolls to bottom
while running.

### killing a command

`ctrl+c` calls `cancel()` on the active command's context, which sends
SIGKILL to the process group. a `buildDoneMsg` arrives with exit code -1.

### command history

- previous commands are stored in `history []string`
- `↑` in the input cycles through previous commands (most recent first)
- `↓` moves forward / back to empty
- history persists in `~/.rig/build_history` (one command per line,
  last 100)

### viewport content

the viewport shows all runs concatenated:

```
$ {command}
{output lines...}
(exit {code}, {duration})

$ {next command}
...
```

styling:
- `$` prompt in purple
- exit 0 in green, non-zero in red
- output in default terminal color
- currently running command shows a spinner instead of exit code

### clear

`ctrl+l` clears all previous runs and resets the viewport.

## keybindings

| key       | action                              |
|-----------|-------------------------------------|
| `enter`   | run command                         |
| `ctrl+c`  | kill running command                |
| `ctrl+l`  | clear output                        |
| `↑`       | previous command from history       |
| `↓`       | next command / clear input          |
| `esc`     | clear input (if not running)        |

## plan integration

the build pane already receives `ActivePlanChangedMsg`. the plan title
is displayed in the header. future enhancement: suggest commands based
on the active plan's tasks (e.g. if a task mentions "run tests", offer
`go test ./...`).

## implementation steps

1. **command execution engine**
   - spawn commands via `$SHELL -c`
   - stream stdout+stderr via tea messages
   - record exit code and duration
   - support cancel via context

2. **build pane: input + viewport**
   - textinput for command entry
   - viewport for scrollable output
   - render runs with styling (exit codes, duration, spinner)

3. **build pane: command history**
   - store commands in memory + persist to `~/.rig/build_history`
   - up/down arrow navigation

4. **build pane: kill + clear**
   - ctrl+c kills active process
   - ctrl+l clears runs

5. **wire into main.go**
   - no constructor changes needed (already takes no args)

## files touched

- `internal/pane/build/build.go` — full rewrite from current stub
- `cmd/rig/main.go` — no changes needed

## future considerations

- run commands in a specific working directory (configurable)
- multiple concurrent commands (background jobs)
- detect project type and suggest common commands (go build, npm run, cargo build)
- integrate with plan: auto-mark tasks done when relevant builds succeed
