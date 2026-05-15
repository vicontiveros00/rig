# git pane — repo status, diffs, and push

## overview

replace the git pane stub with an interactive git interface. the user
sees the current repo status, scrollable diffs of changed files, and
can stage, commit, and push without leaving rig.

relates to: https://github.com/vicontiveros00/rig/issues/4

## goals

- show repo status at a glance (branch, ahead/behind, changed files)
- scrollable unified diff view for any changed file
- stage/unstage individual files
- commit with an inline message input
- push to remote
- pull from remote
- branch switching
- auto-refresh status on pane focus

## design

### views

the git pane has three views toggled by context:

1. **status view** (default) — file list with status indicators
2. **diff view** — scrollable unified diff for the selected file
3. **log view** — recent commit history

### status view layout

```
┌─────────────────────────────────────────────────────────────┐
│  git: main  ↑2  origin/main                                │
│─────────────────────────────────────────────────────────────│
│  staged:                                                     │
│    M internal/pane/build/build.go                            │
│    A internal/pane/build/parse.go                            │
│                                                             │
│  unstaged:                                                   │
│    M internal/app/app.go                                     │
│    M cmd/rig/main.go                                         │
│                                                             │
│  untracked:                                                  │
│    ? specs/build-pane.md                                     │
│                                                             │
│  enter=diff  a=stage  u=unstage  c=commit  p=push  P=pull  │
└─────────────────────────────────────────────────────────────┘
```

- files grouped by staged/unstaged/untracked
- status codes: M=modified, A=added, D=deleted, ?=untracked
- cursor navigates file list
- color: green for staged, red for unstaged, gray for untracked

### diff view layout

```
┌─────────────────────────────────────────────────────────────┐
│  diff: internal/pane/build/build.go                         │
│─────────────────────────────────────────────────────────────│
│  @@ -11,6 +11,8 @@ type Pane struct {                       │
│       width  int                                             │
│       height int                                             │
│  +    runner *exec.Cmd                                       │
│  +    cancel context.CancelFunc                              │
│       ...                                                    │
│                                                             │
│  ↑/↓=scroll  esc=back  a=stage file                        │
└─────────────────────────────────────────────────────────────┘
```

- standard unified diff with color: green for additions, red for deletions
- scrollable viewport
- `esc` returns to status view
- `a` stages the file being viewed

### log view layout

```
┌─────────────────────────────────────────────────────────────┐
│  git log: main                                              │
│─────────────────────────────────────────────────────────────│
│  645564c  extract chatcore package, refine prompts          │
│  ce9060f  add plan pane, chat persistence, tool-call apply  │
│  96fb951  add servers pane and mcp pane                     │
│  850a40d  fix config save clobbering user edits             │
│  74b19df  add version display and build-time injection      │
│                                                             │
│  ↑/↓=scroll  esc=back  l=toggle log                        │
└─────────────────────────────────────────────────────────────┘
```

### state

```go
type gitView int
const (
    viewStatus gitView = iota
    viewDiff
    viewLog
)

type fileEntry struct {
    path   string
    status string // "M", "A", "D", "?"
    staged bool
}

type Pane struct {
    view      gitView
    branch    string
    remote    string
    ahead     int
    behind    int
    files     []fileEntry
    cursor    int
    diffVP    viewport.Model
    logVP     viewport.Model
    statusVP  viewport.Model

    commitMode bool
    commitInput textinput.Model

    width  int
    height int
}
```

### git operations

all git operations run via `exec.Command("git", ...)` with output
captured. operations:

| action | command | trigger |
|--------|---------|---------|
| status | `git status --porcelain=v1 -b` | on init, on focus, after actions |
| diff | `git diff [--cached] -- {file}` | enter on a file |
| stage | `git add {file}` | `a` on a file |
| unstage | `git reset HEAD -- {file}` | `u` on a file |
| stage all | `git add -A` | `A` |
| commit | `git commit -m "{msg}"` | `c` → type message → enter |
| push | `git push` | `p` |
| pull | `git pull` | `P` |
| log | `git log --oneline -20` | `l` |
| branch | `git branch` | future |

### commit flow

1. user presses `c`
2. inline textinput appears at the bottom: "commit message:"
3. user types message, presses `enter`
4. runs `git commit -m "{message}"`
5. auto-refreshes status
6. shows brief confirmation "committed: {short hash}"

### push/pull

- `p` runs `git push`, shows result (success or error)
- `P` runs `git pull`, shows result, auto-refreshes status
- errors display inline in red

### auto-refresh

status refreshes automatically:
- on pane `Init()`
- after any git action (stage, commit, push, pull)
- could optionally refresh on pane focus (when tab switches to git)

## keybindings

### status view
| key     | action                          |
|---------|---------------------------------|
| `↑/↓`  | move through file list          |
| `enter` | view diff for selected file     |
| `a`    | stage selected file              |
| `u`    | unstage selected file            |
| `A`    | stage all                        |
| `c`    | commit (opens message input)     |
| `p`    | push to remote                   |
| `P`    | pull from remote                 |
| `l`    | toggle log view                  |
| `r`    | refresh status                   |

### diff view
| key     | action                          |
|---------|---------------------------------|
| `↑/↓`  | scroll diff                     |
| `a`    | stage this file                  |
| `esc`  | back to status view              |

### log view
| key     | action                          |
|---------|---------------------------------|
| `↑/↓`  | scroll log                      |
| `esc`  | back to status view              |

## implementation steps

1. **git operations helper**
   - functions to run git commands and parse output
   - parse `git status --porcelain` into file entries
   - parse branch/remote/ahead/behind from status header

2. **status view**
   - render file list grouped by staged/unstaged/untracked
   - cursor navigation
   - stage/unstage actions with auto-refresh

3. **diff view**
   - run `git diff` for selected file
   - render in viewport with color (green/red for +/-)
   - stage from diff view

4. **commit + push + pull**
   - inline textinput for commit message
   - run git commands, show results
   - auto-refresh after each action

5. **log view**
   - run `git log --oneline`
   - scrollable viewport

## files touched

- `internal/pane/git/git.go` — full rewrite from stub

## future considerations

- branch creation and switching
- interactive staging (hunk-level)
- stash management
- merge conflict resolution UI
- integration with build pane (auto-commit after successful builds)
