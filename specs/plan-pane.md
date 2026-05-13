# plan pane — structured task planning with cross-pane context

## overview

replace the plan pane stub with a full task-planning interface. the
user creates, edits, and tracks tasks within a plan. the active plan
is persisted to disk and its content is injected as context into the
chat pane's system prompt and made available to the build pane, so
the llm and build commands are always aware of what the user is
working on.

relates to: https://github.com/vicontiveros00/rig/issues/1

## goals

- create and manage hierarchical tasks (title, status, optional notes)
- persist plans to `~/.rig/history/plan/` (same pattern as chat history)
- one plan is "active" at a time — its content flows into chat and build
- switch between saved plans with `ctrl+o`, start new with `ctrl+n`
- simple keyboard-driven ui: add, edit, toggle status, delete, reorder

## plan data model

```go
type Task struct {
    ID       string `json:"id"`
    Title    string `json:"title"`
    Status   string `json:"status"`   // "pending", "in_progress", "done"
    Notes    string `json:"notes"`
    Children []Task `json:"children,omitempty"`
}

type Plan struct {
    ID        string    `json:"id"`
    Title     string    `json:"title"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Tasks     []Task    `json:"tasks"`
}
```

statuses: `pending` → `in_progress` → `done` (cycle with `space`)

## storage

plans live at `~/.rig/history/plan/`:

```
~/.rig/history/plan/
  2026-05-13T21-31-00_add-mcp-support.json
  2026-05-12T10-00-00_initial-setup.json
```

the active plan id is stored in `~/.rig/active_plan` (a single-line
text file) so it persists across rig restarts.

### history package additions

add to `internal/history/history.go`:

- `SavePlan(plan Plan) error`
- `ListPlans() ([]PlanMeta, error)` — id, title, created_at, task count, completion %
- `LoadPlan(id string) (Plan, error)`
- `SetActivePlan(id string) error`
- `GetActivePlan() (string, error)`

## plan pane ui

file: `internal/pane/plan/plan.go`

layout:
```
┌─────────────────────────────────────────────────────────────┐
│  plan: add mcp support                          3/7 tasks   │
│                                                             │
│  ○ implement mcp client package                             │
│  ● implement mcp pane ui                         in progress│
│    notes: sse transport working, need stdio                 │
│  ✓ wire into main.go                                  done  │
│  ○ add health check to servers pane                         │
│    ○ http endpoint check                                    │
│    ○ stdio spawn check                                      │
│  ○ test with searxng                                        │
│  ○ update docs                                              │
│                                                             │
│  a=add  e=edit  space=toggle  d=del  n=notes  ctrl+n=new   │
└─────────────────────────────────────────────────────────────┘
```

- tasks are displayed as a flat or indented list
- status indicators: `○` pending, `●` in progress, `✓` done
- cursor highlights the selected task
- completion count shown in the header

### task editing

`a` opens an inline text input to add a new task after the cursor.
`e` opens an inline text input pre-filled with the selected task's title.
`n` opens a multi-line input for task notes.
all save on `enter`, cancel on `esc`.

### child tasks

`tab` indents the selected task (makes it a child of the task above).
`shift+tab` un-indents. maximum depth: 2 levels.

## cross-pane context: active plan

the key feature: the active plan's content is shared with other panes.

### message type

add to `internal/messages/messages.go`:

```go
type ActivePlanChangedMsg struct {
    Plan *plan.Plan  // nil if no active plan
}
```

the plan pane emits this whenever the plan is modified or switched.
the app root broadcasts it to all panes.

### chat pane integration

when the chat pane receives `ActivePlanChangedMsg`, it stores the
plan reference. the system prompt in `startStream` is augmented:

```
You are Rigby...

## Active Plan
The user is currently working on: "add mcp support"
Tasks:
- [ ] implement mcp client package
- [x] implement mcp pane ui (in progress)
  notes: sse transport working, need stdio
- [x] wire into main.go (done)
- [ ] add health check to servers pane
...
```

this gives the llm full awareness of what the user is working on
without the user needing to re-explain it every conversation.

### build pane integration

the build pane (once implemented per issue #2) will receive the same
`ActivePlanChangedMsg`. it can use the active plan to:
- suggest build commands relevant to the current task
- display which plan task a build relates to
- auto-mark tasks as done when relevant builds succeed

for now, just store the plan reference in the build pane struct so
it's ready when the build pane is implemented.

## keybindings

| key          | action                                       |
|--------------|----------------------------------------------|
| `↑/↓`       | move cursor through task list                |
| `a`          | add new task after cursor                    |
| `e`          | edit selected task title                     |
| `space`      | cycle status: pending → in_progress → done   |
| `d`          | delete selected task (with confirmation)     |
| `n`          | edit notes on selected task                  |
| `tab`        | indent task (make child)                     |
| `shift+tab`  | un-indent task                               |
| `ctrl+n`     | new plan (archives current)                  |
| `ctrl+o`     | open plan picker                             |
| `ctrl+s`     | save current plan                            |

## auto-save

the plan auto-saves after every modification (add, edit, toggle,
delete, reorder). no explicit save needed, but `ctrl+s` is there
for reassurance.

## implementation steps

1. **history: add plan persistence**
   - add `Plan`, `Task`, `PlanMeta` types to history package
   - implement `SavePlan`, `ListPlans`, `LoadPlan`
   - implement `SetActivePlan`, `GetActivePlan`

2. **messages: add `ActivePlanChangedMsg`**
   - add message type
   - app root broadcasts to all panes

3. **plan pane: task list + editing**
   - replace stub with real pane
   - state: plan, cursor, editing mode
   - render task list with status indicators
   - implement add/edit/delete/toggle/notes
   - implement child task indentation
   - auto-save on every change
   - emit `ActivePlanChangedMsg` on changes

4. **plan pane: plan management**
   - `ctrl+n` archives and creates new plan
   - `ctrl+o` opens plan picker (reuse pattern from chat)
   - load active plan from `~/.rig/active_plan` on startup

5. **chat pane: consume active plan context**
   - handle `ActivePlanChangedMsg`
   - inject plan summary into system prompt
   - format as markdown task list

6. **build pane: store plan reference**
   - handle `ActivePlanChangedMsg`
   - store plan pointer for future use (build pane is still a stub)

## files touched

- `internal/history/history.go` — add plan persistence functions
- `internal/messages/messages.go` — add `ActivePlanChangedMsg`
- `internal/pane/plan/plan.go` — full rewrite from stub
- `internal/pane/chat/chat.go` — handle `ActivePlanChangedMsg`, augment system prompt
- `internal/pane/build/build.go` — handle `ActivePlanChangedMsg` (store reference)
- `internal/app/app.go` — broadcast `ActivePlanChangedMsg`
- `cmd/rig/main.go` — pass config to plan pane if needed

## open questions

- should the plan context in chat be opt-in (toggle) or always-on?
- should completed tasks be hidden/collapsed in the plan view?
- should there be a way to generate a plan from a chat conversation
  (e.g. "create a plan from this discussion")?
