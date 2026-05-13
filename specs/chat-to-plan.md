# plan-mode chat — conversational planning inside the plan pane

## overview

add a built-in chat interface to the plan pane. the user toggles
between the task list view and a plan-focused chat where Rigby has
full context of the active plan. Rigby can propose task changes
directly, and the user applies them without leaving the pane.

the general chat pane remains unchanged — it has read-only visibility
of the active plan via the system prompt. the plan pane owns the
two-way flow.

relates to: https://github.com/vicontiveros00/rig/issues/11

## goals

- plan pane has two modes: **list mode** (current) and **chat mode**
- in chat mode, Rigby sees the full plan and can propose changes
- proposals use fenced `plan` blocks that get parsed and applied
- the user explicitly applies proposals — no silent mutations
- chat history within the plan pane is scoped to the active plan

## design

### mode toggle

`c` in list mode opens plan chat. `esc` in chat mode returns to list.

```
list mode:                         chat mode:
┌──────────────────────┐          ┌──────────────────────┐
│ plan: add mcp support│          │ plan: add mcp support│
│                      │          │                      │
│ ○ implement client   │   c →    │ you                  │
│ ● implement ui       │          │ expand step 2        │
│ ✓ wire into main     │          │                      │
│                      │          │ rigby                 │
│ a=add  e=edit  c=chat│          │ here's the breakdown │
│                      │   ← esc  │ ```plan              │
│                      │          │ - [ ] sse transport   │
│                      │          │ - [ ] tool list view  │
│                      │          │ ```                   │
│                      │          │                      │
│                      │          │ > type a message...   │
│                      │          │ ctrl+p=apply  esc=back│
└──────────────────────┘          └──────────────────────┘
```

### plan chat system prompt

the plan chat uses a specialized system prompt:

```
You are Rigby, a planning assistant inside rig. You are in the plan
pane helping the user work on their plan.

## Current Plan: "{title}"

{full markdown task list}

When the user asks you to create, expand, or modify tasks, output
your proposed changes in a fenced ```plan block:
- [ ] pending task
- [~] in-progress task
- [x] completed task
  - [ ] subtask (2-space indent)
  notes: optional notes line

The user will press ctrl+p to apply your proposal to the plan.
Keep responses focused on planning — be concise and action-oriented.
```

### plan chat state

the plan chat maintains its own message history, separate from the
main chat pane. messages are scoped to the active plan — switching
plans clears the chat. the plan chat does not persist to disk (it's
ephemeral, focused on the current planning session).

```go
type planChatState struct {
    messages  []chat.Message
    input     textarea.Model
    viewport  viewport.Model
    spinner   spinner.Model
    streaming bool
    streamCh  <-chan llm.StreamChunk
    cancel    context.CancelFunc
}
```

### applying proposals

in chat mode, `ctrl+p`:
1. scans the last assistant message for a fenced `plan` block
2. parses it via `ParsePlanMarkdown` into `[]history.Task`
3. if current plan has no tasks → replace
4. if current plan has tasks → append
5. auto-save, emit `ActivePlanChangedMsg`
6. show a brief "applied N tasks" confirmation

### plan chat needs the llm provider

the plan pane currently doesn't have access to the llm provider.
the constructor needs to accept the provider + model, and handle
`ModelSelectedMsg` to stay in sync.

update `plan.New()` signature:

```go
func New(provider llm.Provider, model string) pane.Pane
```

update `cmd/rig/main.go` to pass them.

## format contract

same as `formatPlanMarkdown` already produces:

- `- [ ]` = pending
- `- [~]` = in progress
- `- [x]` = done
- 2-space indentation = child task
- `notes:` after a task = task notes

## parsing

new file: `internal/pane/plan/parse.go`

```go
func ParsePlanMarkdown(input string) []history.Task
```

- each `- [` line is a task
- indentation determines depth
- `notes:` lines attach to preceding task
- auto-generate task IDs
- skip blank/unrecognized lines

## implementation steps

1. **parser: `ParsePlanMarkdown`**
   - new file `internal/pane/plan/parse.go`
   - handle markers, indentation, notes

2. **plan pane: add chat state + mode toggle**
   - add `planChatState` to pane struct
   - `c` enters chat mode, `esc` returns to list
   - chat mode has its own textarea, viewport, spinner
   - specialized system prompt with full plan context

3. **plan pane: streaming**
   - accept `llm.Provider` + model in constructor
   - handle `ModelSelectedMsg` to stay in sync
   - stream responses same as chat pane

4. **plan pane: `ctrl+p` apply**
   - extract last `plan` block from last assistant message
   - parse, apply to plan (replace if empty, append if not)
   - auto-save, emit `ActivePlanChangedMsg`
   - show confirmation

5. **wire: update main.go**
   - pass provider + model to `plan.New()`

## keybindings (updated)

### list mode
| key          | action                          |
|--------------|---------------------------------|
| `↑/↓`       | move through tasks              |
| `a`          | add task                        |
| `e`          | edit task title                 |
| `space`      | cycle task status               |
| `d`          | delete task                     |
| `n`          | edit task notes                 |
| `E`          | rename plan                     |
| `c`          | enter plan chat                 |
| `ctrl+n`     | new plan                        |
| `ctrl+o`     | plan picker                     |

### chat mode
| key          | action                          |
|--------------|---------------------------------|
| `enter`      | send message                    |
| `alt+enter`  | newline in input                |
| `ctrl+p`     | apply last plan block           |
| `esc`        | return to list mode             |

## files touched

- `internal/pane/plan/parse.go` — new, markdown plan parser
- `internal/pane/plan/plan.go` — add chat mode, provider, streaming
- `cmd/rig/main.go` — pass provider + model to plan pane
