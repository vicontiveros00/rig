package git

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/pane"
	"github.com/vicontiveros00/rig/internal/project"
)

type gitView int

const (
	viewStatus gitView = iota
	viewDiff
	viewLog
	viewBranches
	viewClone
)

type branchEntry struct {
	name    string
	remote  bool
	current bool
}

type fileEntry struct {
	path   string
	status string // "M", "A", "D", "R", "?"
	staged bool
}

type statusRefreshedMsg struct {
	branch  string
	remote  string
	ahead   int
	behind  int
	files   []fileEntry
	hasRepo bool
}

type diffLoadedMsg struct {
	content string
}

type logLoadedMsg struct {
	content string
}

type branchesLoadedMsg struct {
	branches []branchEntry
}

type gitActionDoneMsg struct {
	output string
	err    error
}

type Pane struct {
	view   gitView
	width  int
	height int

	hasRepo bool
	branch  string
	remote  string
	ahead   int
	behind  int
	files   []fileEntry
	cursor  int

	statusVP  viewport.Model
	diffVP    viewport.Model
	logVP     viewport.Model
	diffFile  string

	commitMode  bool
	commitInput textinput.Model

	branches    []branchEntry
	branchVP    viewport.Model
	branchIdx   int

	cloneInput textinput.Model
	actionMsg  string
	actionErr  bool
}

func New() pane.Pane {
	ci := textinput.New()
	ci.Placeholder = "commit message"

	cli := textinput.New()
	cli.Placeholder = "repo url (e.g. https://github.com/user/repo.git)"
	cli.Focus()

	p := &Pane{
		commitInput: ci,
		cloneInput:  cli,
	}
	return p
}

func (p *Pane) Name() string      { return "git" }
func (p *Pane) ShortHelp() string { return "status, diff, commit, push" }

func (p *Pane) SetSize(w, h int) {
	p.width = w
	p.height = h
	vpH := h - 5
	if vpH < 1 {
		vpH = 1
	}
	p.statusVP.Width = w
	p.statusVP.Height = vpH
	p.diffVP.Width = w
	p.diffVP.Height = vpH
	p.logVP.Width = w
	p.logVP.Height = vpH
	p.branchVP.Width = w
	p.branchVP.Height = vpH
	p.commitInput.Width = w - 20
	p.cloneInput.Width = w - 10
}

func (p *Pane) Init() tea.Cmd {
	return p.refreshStatus()
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case statusRefreshedMsg:
		p.hasRepo = msg.hasRepo
		p.branch = msg.branch
		p.remote = msg.remote
		p.ahead = msg.ahead
		p.behind = msg.behind
		p.files = msg.files
		if !p.hasRepo {
			p.view = viewClone
		}
		p.updateStatusViewport()
		return p, nil

	case diffLoadedMsg:
		p.view = viewDiff
		p.diffVP.SetContent(colorizeDiff(msg.content))
		p.diffVP.GotoTop()
		return p, nil

	case logLoadedMsg:
		p.view = viewLog
		p.logVP.SetContent(msg.content)
		p.logVP.GotoTop()
		return p, nil

	case branchesLoadedMsg:
		p.branches = msg.branches
		p.branchIdx = 0
		p.view = viewBranches
		p.updateBranchViewport()
		return p, nil

	case gitActionDoneMsg:
		if msg.err != nil {
			p.actionMsg = msg.err.Error()
			p.actionErr = true
		} else {
			p.actionMsg = msg.output
			p.actionErr = false
		}
		return p, p.refreshStatus()

	case tea.KeyMsg:
		switch p.view {
		case viewClone:
			return p.updateClone(msg)
		case viewDiff:
			return p.updateDiff(msg)
		case viewLog:
			return p.updateLog(msg)
		case viewBranches:
			return p.updateBranches(msg)
		default:
			if p.commitMode {
				return p.updateCommit(msg)
			}
			return p.updateStatus(msg)
		}
	}

	return p, nil
}

func (p *Pane) updateStatus(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
			p.updateStatusViewport()
			p.scrollStatusToCursor()
		}
	case "down", "j":
		if p.cursor < len(p.files)-1 {
			p.cursor++
			p.updateStatusViewport()
			p.scrollStatusToCursor()
		}
	case "enter":
		if len(p.files) > 0 {
			f := p.files[p.cursor]
			p.diffFile = f.path
			return p, p.loadDiff(f)
		}
	case "a":
		if len(p.files) > 0 {
			f := p.files[p.cursor]
			return p, p.gitCmd("add", f.path)
		}
	case "u":
		if len(p.files) > 0 {
			f := p.files[p.cursor]
			return p, p.gitCmd("reset", "HEAD", "--", f.path)
		}
	case "A":
		return p, p.gitCmd("add", "-A")
	case "c":
		p.commitMode = true
		p.commitInput.SetValue("")
		p.commitInput.Focus()
	case "p":
		return p, p.gitCmdAsync("push")
	case "P":
		return p, p.gitCmdAsync("pull")
	case "s":
		return p, p.gitCmdAsync("stash")
	case "S":
		return p, p.gitCmdAsync("stash", "pop")
	case "l":
		return p, p.loadLog()
	case "b":
		return p, p.loadBranches()
	case "f":
		return p, p.gitCmdAsync("fetch", "--all")
	case "r":
		return p, p.refreshStatus()
	}
	return p, nil
}

func (p *Pane) updateCommit(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.commitMode = false
		p.commitInput.Blur()
		return p, nil
	case "enter":
		message := strings.TrimSpace(p.commitInput.Value())
		if message == "" {
			return p, nil
		}
		p.commitMode = false
		p.commitInput.Blur()
		return p, p.gitCmdAsync("commit", "-m", message)
	}

	var cmd tea.Cmd
	p.commitInput, cmd = p.commitInput.Update(msg)
	return p, cmd
}

func (p *Pane) updateDiff(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.view = viewStatus
		return p, nil
	case "a":
		return p, p.gitCmd("add", p.diffFile)
	}

	var cmd tea.Cmd
	p.diffVP, cmd = p.diffVP.Update(msg)
	return p, cmd
}

func (p *Pane) updateLog(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.view = viewStatus
		return p, nil
	}

	var cmd tea.Cmd
	p.logVP, cmd = p.logVP.Update(msg)
	return p, cmd
}

func (p *Pane) updateBranches(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.view = viewStatus
		return p, nil
	case "up", "k":
		if p.branchIdx > 0 {
			p.branchIdx--
			p.updateBranchViewport()
		}
	case "down", "j":
		if p.branchIdx < len(p.branches)-1 {
			p.branchIdx++
			p.updateBranchViewport()
		}
	case "enter":
		if p.branchIdx < len(p.branches) {
			br := p.branches[p.branchIdx]
			if !br.current {
				name := br.name
				if br.remote {
					parts := strings.SplitN(name, "/", 2)
					if len(parts) == 2 {
						name = parts[1]
					}
				}
				return p, p.gitCmdAsync("checkout", name)
			}
		}
	}

	var cmd tea.Cmd
	p.branchVP, cmd = p.branchVP.Update(msg)
	return p, cmd
}

func (p *Pane) updateClone(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "enter":
		url := strings.TrimSpace(p.cloneInput.Value())
		if url == "" {
			return p, nil
		}
		p.cloneInput.SetValue("")
		return p, p.gitCmdAsync("clone", url, ".")
	}

	var cmd tea.Cmd
	p.cloneInput, cmd = p.cloneInput.Update(msg)
	return p, cmd
}

// --- View ---

func (p *Pane) View() string {
	if p.width == 0 {
		return ""
	}

	switch p.view {
	case viewClone:
		return p.viewClone()
	case viewDiff:
		return p.viewDiffPane()
	case viewLog:
		return p.viewLogPane()
	case viewBranches:
		return p.viewBranchesPane()
	default:
		return p.viewStatusPane()
	}
}

func (p *Pane) viewStatusPane() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))

	branchInfo := p.branch
	if p.ahead > 0 {
		branchInfo += fmt.Sprintf(" ↑%d", p.ahead)
	}
	if p.behind > 0 {
		branchInfo += fmt.Sprintf(" ↓%d", p.behind)
	}
	if p.remote != "" {
		branchInfo += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(p.remote)
	}

	b.WriteString(headerStyle.Render(fmt.Sprintf("  git: %s", branchInfo)))
	b.WriteString("\n")

	b.WriteString(p.statusVP.View())
	b.WriteString("\n")

	if p.commitMode {
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		b.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render("commit:"), p.commitInput.View()))
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	if p.commitMode {
		b.WriteString(help.Render("  enter=commit  esc=cancel"))
	} else {
		b.WriteString(help.Render("  enter=diff  a=stage  u=unstage  A=all  c=commit  p=push  P=pull  s=stash  S=pop  f=fetch  b=branches  l=log"))
	}

	return b.String()
}

func (p *Pane) viewDiffPane() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render(fmt.Sprintf("  diff: %s", p.diffFile)))
	b.WriteString("\n")
	b.WriteString(p.diffVP.View())
	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  ↑/↓=scroll  a=stage  esc=back"))
	return b.String()
}

func (p *Pane) viewLogPane() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render(fmt.Sprintf("  git log: %s", p.branch)))
	b.WriteString("\n")
	b.WriteString(p.logVP.View())
	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  ↑/↓=scroll  esc=back"))
	return b.String()
}

func (p *Pane) viewBranchesPane() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  branches"))
	b.WriteString("\n")
	b.WriteString(p.branchVP.View())
	b.WriteString("\n")
	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  ↑/↓=navigate  enter=checkout  esc=back"))
	return b.String()
}

func (p *Pane) updateBranchViewport() {
	localStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	remoteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))
	currentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)

	var content strings.Builder

	hasLocal := false
	for _, br := range p.branches {
		if !br.remote {
			hasLocal = true
			break
		}
	}
	if hasLocal {
		content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E5E7EB")).Render("  local:"))
		content.WriteString("\n")
	}

	idx := 0
	for i, br := range p.branches {
		if br.remote {
			continue
		}
		var line string
		if br.current {
			line = fmt.Sprintf("  * %s", currentStyle.Render(br.name))
		} else {
			line = fmt.Sprintf("    %s", localStyle.Render(br.name))
		}
		if i == p.branchIdx {
			line = lipgloss.NewStyle().Background(lipgloss.Color("#1E1B4B")).Width(p.width - 2).Render(line)
		}
		content.WriteString(line + "\n")
		idx++
	}

	hasRemote := false
	for _, br := range p.branches {
		if br.remote {
			hasRemote = true
			break
		}
	}
	if hasRemote {
		content.WriteString("\n")
		content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E5E7EB")).Render("  remote:"))
		content.WriteString("\n")
	}

	for i, br := range p.branches {
		if !br.remote {
			continue
		}
		line := fmt.Sprintf("    %s", remoteStyle.Render(br.name))
		if i == p.branchIdx {
			line = lipgloss.NewStyle().Background(lipgloss.Color("#1E1B4B")).Width(p.width - 2).Render(line)
		}
		content.WriteString(line + "\n")
	}

	p.branchVP.SetContent(content.String())
}

func (p *Pane) viewClone() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  git: no repository detected"))
	b.WriteString("\n\n")

	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(dim.Render("  clone a repository to get started:"))
	b.WriteString("\n\n")

	labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
	b.WriteString(fmt.Sprintf("  %s %s", labelStyle.Render("url:"), p.cloneInput.View()))
	b.WriteString("\n\n")

	if p.actionMsg != "" {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
		if p.actionErr {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
		}
		b.WriteString(style.Render("  " + p.actionMsg))
		b.WriteString("\n\n")
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  enter=clone"))

	return b.String()
}

func (p *Pane) updateStatusViewport() {
	var content strings.Builder

	if p.actionMsg != "" {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
		if p.actionErr {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
		}
		content.WriteString(style.Render(formatActionMsg(p.actionMsg, p.actionErr)))
		content.WriteString("\n\n")
	}

	stagedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	unstagedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	untrackedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))

	// Group files
	var staged, unstaged, untracked []fileEntry
	for _, f := range p.files {
		if f.status == "?" {
			untracked = append(untracked, f)
		} else if f.staged {
			staged = append(staged, f)
		} else {
			unstaged = append(unstaged, f)
		}
	}

	idx := 0
	if len(staged) > 0 {
		content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#10B981")).Render("  staged:"))
		content.WriteString("\n")
		for _, f := range staged {
			line := fmt.Sprintf("    %s %s", stagedStyle.Render(f.status), f.path)
			if idx == p.cursor {
				line = lipgloss.NewStyle().Background(lipgloss.Color("#1E1B4B")).Width(p.width - 2).Render(line)
			}
			content.WriteString(line + "\n")
			idx++
		}
		content.WriteString("\n")
	}

	if len(unstaged) > 0 {
		content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#EF4444")).Render("  unstaged:"))
		content.WriteString("\n")
		for _, f := range unstaged {
			line := fmt.Sprintf("    %s %s", unstagedStyle.Render(f.status), f.path)
			if idx == p.cursor {
				line = lipgloss.NewStyle().Background(lipgloss.Color("#1E1B4B")).Width(p.width - 2).Render(line)
			}
			content.WriteString(line + "\n")
			idx++
		}
		content.WriteString("\n")
	}

	if len(untracked) > 0 {
		content.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#6B7280")).Render("  untracked:"))
		content.WriteString("\n")
		for _, f := range untracked {
			line := fmt.Sprintf("    %s %s", untrackedStyle.Render("?"), f.path)
			if idx == p.cursor {
				line = lipgloss.NewStyle().Background(lipgloss.Color("#1E1B4B")).Width(p.width - 2).Render(line)
			}
			content.WriteString(line + "\n")
			idx++
		}
	}

	if len(p.files) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		content.WriteString(dim.Render("  working tree clean"))
	}

	p.statusVP.SetContent(content.String())
}

// --- Git commands ---

func (p *Pane) refreshStatus() tea.Cmd {
	return func() tea.Msg {
		_, hasProject := project.DetectRoot()
		if !hasProject {
			return statusRefreshedMsg{hasRepo: false}
		}

		out, err := exec.Command("git", "status", "--porcelain=v1", "-b").Output()
		if err != nil {
			return statusRefreshedMsg{hasRepo: false}
		}

		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		msg := statusRefreshedMsg{hasRepo: true}

		if len(lines) > 0 && strings.HasPrefix(lines[0], "## ") {
			msg.branch, msg.remote, msg.ahead, msg.behind = parseBranchLine(lines[0])
			lines = lines[1:]
		}

		for _, line := range lines {
			if len(line) < 4 {
				continue
			}
			x, y := line[0], line[1]
			path := strings.TrimSpace(line[3:])

			if x == '?' && y == '?' {
				msg.files = append(msg.files, fileEntry{path: path, status: "?", staged: false})
			} else {
				if x != ' ' && x != '?' {
					msg.files = append(msg.files, fileEntry{path: path, status: string(x), staged: true})
				}
				if y != ' ' && y != '?' {
					msg.files = append(msg.files, fileEntry{path: path, status: string(y), staged: false})
				}
			}
		}

		return msg
	}
}

func parseBranchLine(line string) (branch, remote string, ahead, behind int) {
	line = strings.TrimPrefix(line, "## ")

	if idx := strings.Index(line, "..."); idx >= 0 {
		branch = line[:idx]
		rest := line[idx+3:]
		if spaceIdx := strings.IndexByte(rest, ' '); spaceIdx >= 0 {
			remote = rest[:spaceIdx]
			info := rest[spaceIdx:]
			fmt.Sscanf(info, " [ahead %d", &ahead)
			fmt.Sscanf(info, " [behind %d", &behind)
			if strings.Contains(info, "ahead") && strings.Contains(info, "behind") {
				fmt.Sscanf(info, " [ahead %d, behind %d]", &ahead, &behind)
			}
		} else {
			remote = rest
		}
	} else {
		branch = strings.Fields(line)[0]
	}
	return
}

func (p *Pane) loadDiff(f fileEntry) tea.Cmd {
	return func() tea.Msg {
		var args []string
		if f.staged {
			args = []string{"diff", "--cached", "--", f.path}
		} else if f.status == "?" {
			args = []string{"diff", "--no-index", "/dev/null", f.path}
		} else {
			args = []string{"diff", "--", f.path}
		}
		out, _ := exec.Command("git", args...).Output()
		return diffLoadedMsg{content: string(out)}
	}
}

func (p *Pane) loadLog() tea.Cmd {
	return func() tea.Msg {
		out, _ := exec.Command("git", "log", "--oneline", "-30").Output()
		return logLoadedMsg{content: string(out)}
	}
}

func (p *Pane) loadBranches() tea.Cmd {
	return func() tea.Msg {
		var branches []branchEntry

		// Local branches
		out, _ := exec.Command("git", "branch", "--format=%(refname:short) %(HEAD)").Output()
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			name := parts[0]
			current := len(parts) > 1 && parts[1] == "*"
			branches = append(branches, branchEntry{name: name, current: current})
		}

		// Remote branches
		out, _ = exec.Command("git", "branch", "-r", "--format=%(refname:short)").Output()
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" || strings.Contains(line, "HEAD") {
				continue
			}
			branches = append(branches, branchEntry{name: line, remote: true})
		}

		return branchesLoadedMsg{branches: branches}
	}
}

func (p *Pane) gitCmd(args ...string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			return gitActionDoneMsg{output: string(out), err: fmt.Errorf("%s", strings.TrimSpace(string(out)))}
		}
		return gitActionDoneMsg{output: strings.TrimSpace(string(out))}
	}
}

func (p *Pane) gitCmdAsync(args ...string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			return gitActionDoneMsg{output: string(out), err: fmt.Errorf("%s", strings.TrimSpace(string(out)))}
		}
		return gitActionDoneMsg{output: strings.TrimSpace(string(out))}
	}
}

// --- Helpers ---

func colorizeDiff(diff string) string {
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#3B82F6"))

	var sb strings.Builder
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			sb.WriteString(delStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line))
		default:
			sb.WriteString(line)
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func (p *Pane) scrollStatusToCursor() {
	// Estimate the line offset: action msg takes ~2 lines, each section header ~2 lines
	// Each file entry is 1 line. This is approximate but good enough.
	lineOffset := 0
	if p.actionMsg != "" {
		lineOffset += 2
	}

	staged, unstaged, untracked := 0, 0, 0
	for _, f := range p.files {
		if f.status == "?" {
			untracked++
		} else if f.staged {
			staged++
		} else {
			unstaged++
		}
	}

	idx := p.cursor
	if staged > 0 {
		lineOffset++ // "staged:" header
		if idx < staged {
			lineOffset += idx
		} else {
			lineOffset += staged + 1 // all staged + blank line
			idx -= staged
		}
	}
	if unstaged > 0 && idx >= 0 {
		lineOffset++ // "unstaged:" header
		if idx < unstaged {
			lineOffset += idx
		} else {
			lineOffset += unstaged + 1
			idx -= unstaged
		}
	}
	if untracked > 0 && idx >= 0 {
		lineOffset++ // "untracked:" header
		lineOffset += idx
	}

	if lineOffset < p.statusVP.YOffset {
		p.statusVP.SetYOffset(lineOffset)
	} else if lineOffset >= p.statusVP.YOffset+p.statusVP.Height {
		p.statusVP.SetYOffset(lineOffset - p.statusVP.Height + 1)
	}
}

func formatActionMsg(msg string, isErr bool) string {
	if !isErr {
		if nl := strings.IndexByte(msg, '\n'); nl >= 0 {
			msg = msg[:nl]
		}
		return "  " + msg
	}

	lower := strings.ToLower(msg)

	switch {
	case strings.Contains(lower, "would be overwritten by merge"):
		return "  pull failed: local changes conflict. press s to stash, then P to pull, then S to pop"
	case strings.Contains(lower, "not a git repository"):
		return "  not a git repository"
	case strings.Contains(lower, "nothing to commit"):
		return "  nothing to commit — working tree clean"
	case strings.Contains(lower, "rejected") && strings.Contains(lower, "push"):
		return "  push rejected: remote has changes. pull first (P), then push (p)"
	case strings.Contains(lower, "conflict"):
		return "  merge conflict detected — resolve conflicts, stage files (a), then commit (c)"
	case strings.Contains(lower, "permission denied") || strings.Contains(lower, "authentication"):
		return "  authentication failed — check your credentials or SSH key"
	default:
		first := msg
		if nl := strings.IndexByte(first, '\n'); nl >= 0 {
			first = first[:nl]
		}
		if len(first) > 80 {
			first = first[:77] + "..."
		}
		return "  " + first
	}
}
