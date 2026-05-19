package build

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/chatcore"
	"github.com/vicontiveros00/rig/internal/llm"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
	"github.com/vicontiveros00/rig/internal/project"
)

type viewMode int

const (
	modeManual viewMode = iota
	modeAgent
)

type agentState int

const (
	agentWaiting agentState = iota
	agentStreaming
	agentPending
	agentRunning
	agentDone
	agentStuck
)

type commandRun struct {
	cmd      string
	output   strings.Builder
	exitCode int
	duration time.Duration
	running  bool
}

type buildOutputMsg struct {
	line string
}

type buildDoneMsg struct {
	exitCode int
	duration time.Duration
}

type Pane struct {
	mode   viewMode
	width  int
	height int

	// Manual mode
	runs       []commandRun
	cmdInput   textinput.Model
	outputVP   viewport.Model
	proc       *exec.Cmd
	cmdCancel  context.CancelFunc
	cmdRunning bool

	// Command history
	history    []string
	historyIdx int

	// Agent mode
	agent       chatcore.Engine
	agentState  agentState
	agentInput  textarea.Model
	agentVP     viewport.Model
	agentSpin   spinner.Model
	pendingCmd  string
	agentDoneMsg string
	agentStuckMsg string

	// Plan context
	activePlanTitle string
	activePlanTasks string

	// Project context
	projectRoot string
	projectTree string
	pendingFile string
}

func New(provider llm.Provider, model string) pane.Pane {
	ci := textinput.New()
	ci.Placeholder = "command (e.g. go build ./...)"
	ci.Prompt = "$ "
	ci.Focus()

	ai := textarea.New()
	ai.Placeholder = "tell Rigby what to build..."
	ai.ShowLineNumbers = false
	ai.SetHeight(3)
	ai.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	root, hasProject := project.DetectRoot()
	var tree string
	if hasProject {
		tree = project.Tree(root, 4)
	}

	p := &Pane{
		cmdInput:    ci,
		agentInput:  ai,
		agentSpin:   sp,
		agent:       chatcore.Engine{Provider: provider, Model: model},
		projectRoot: root,
		projectTree: tree,
	}
	p.loadHistory()
	return p
}

func (p *Pane) Name() string      { return "build" }
func (p *Pane) ShortHelp() string { return "run builds and commands" }

func (p *Pane) SetSize(w, h int) {
	p.width = w
	p.height = h

	p.cmdInput.Width = w - 4
	p.outputVP.Width = w
	p.outputVP.Height = h - 4

	p.agentInput.SetWidth(w - 2)
	p.agentInput.SetHeight(3)
	p.agentVP.Width = w
	p.agentVP.Height = h - 6
}

func (p *Pane) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, p.agentSpin.Tick)
}

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ModelSelectedMsg:
		p.agent.SetProvider(msg.Provider, msg.Model)
		return p, nil

	case messages.ActivePlanChangedMsg:
		p.activePlanTitle = msg.PlanTitle
		p.activePlanTasks = msg.PlanTasks
		return p, nil

	case buildOutputMsg:
		if len(p.runs) > 0 {
			last := &p.runs[len(p.runs)-1]
			last.output.WriteString(msg.line + "\n")
		}
		p.updateOutputViewport()
		return p, nil

	case buildDoneMsg:
		if len(p.runs) > 0 {
			last := &p.runs[len(p.runs)-1]
			last.running = false
			last.exitCode = msg.exitCode
			last.duration = msg.duration
		}
		p.cmdRunning = false
		p.updateOutputViewport()

		if p.mode == modeAgent && p.agentState == agentRunning {
			return p, p.feedOutputToAgent()
		}
		return p, nil

	case chatcore.StreamReadyMsg:
		cmd, err := p.agent.HandleReady(msg)
		if err != nil {
			p.agentState = agentWaiting
			p.updateAgentViewport()
			return p, nil
		}
		return p, cmd

	case chatcore.ChunkMsg:
		done := p.agent.HandleChunk(msg.Chunk)
		p.updateAgentViewport()
		if done {
			p.checkAgentToolCall()
			return p, nil
		}
		return p, p.agent.WaitForChunk()

	case spinner.TickMsg:
		var cmd tea.Cmd
		p.agentSpin, cmd = p.agentSpin.Update(msg)
		return p, cmd

	case tea.KeyMsg:
		if p.mode == modeAgent {
			return p.updateAgent(msg)
		}
		return p.updateManual(msg)
	}

	return p, nil
}

// --- Manual mode ---

func (p *Pane) updateManual(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if p.cmdRunning && p.cmdCancel != nil {
			p.cmdCancel()
			return p, nil
		}
	case "ctrl+l":
		p.runs = nil
		p.updateOutputViewport()
		return p, nil
	case "c":
		if !p.cmdRunning {
			p.mode = modeAgent
			p.agentInput.Focus()
			p.cmdInput.Blur()
			return p, nil
		}
	case "up":
		if len(p.history) > 0 {
			if p.historyIdx > 0 {
				p.historyIdx--
			}
			p.cmdInput.SetValue(p.history[p.historyIdx])
			return p, nil
		}
	case "down":
		if p.historyIdx < len(p.history)-1 {
			p.historyIdx++
			p.cmdInput.SetValue(p.history[p.historyIdx])
		} else {
			p.historyIdx = len(p.history)
			p.cmdInput.SetValue("")
		}
		return p, nil
	case "enter":
		cmd := strings.TrimSpace(p.cmdInput.Value())
		if cmd == "" || p.cmdRunning {
			return p, nil
		}
		p.cmdInput.SetValue("")
		p.addToHistory(cmd)
		return p, p.runCommand(cmd)
	}

	var cmd tea.Cmd
	p.cmdInput, cmd = p.cmdInput.Update(msg)
	return p, cmd
}

func (p *Pane) runCommand(command string) tea.Cmd {
	p.runs = append(p.runs, commandRun{cmd: command, running: true})
	p.cmdRunning = true
	p.updateOutputViewport()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())
		p.cmdCancel = cancel

		start := time.Now()
		proc := exec.CommandContext(ctx, shell, "-c", command)
		p.proc = proc

		stdout, err := proc.StdoutPipe()
		if err != nil {
			return buildDoneMsg{exitCode: -1, duration: time.Since(start)}
		}
		proc.Stderr = proc.Stdout

		if err := proc.Start(); err != nil {
			return buildDoneMsg{exitCode: -1, duration: time.Since(start)}
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			// We can't easily send tea.Msg from here without a program reference,
			// so we accumulate directly
			if len(p.runs) > 0 {
				last := &p.runs[len(p.runs)-1]
				last.output.WriteString(scanner.Text() + "\n")
			}
		}

		exitCode := 0
		if err := proc.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		cancel()
		return buildDoneMsg{exitCode: exitCode, duration: time.Since(start)}
	}
}

// --- Agent mode ---

func (p *Pane) updateAgent(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	if p.agent.Streaming {
		if msg.String() == "esc" {
			p.agent.CancelStream()
			p.agentState = agentWaiting
			return p, nil
		}
		return p, nil
	}

	if p.agentState == agentPending {
		switch msg.String() {
		case "y":
			if p.pendingFile != "" {
				return p, p.approveReadFile()
			}
			cmd := p.pendingCmd
			p.pendingCmd = ""
			p.agentState = agentRunning
			return p, p.runCommand(cmd)
		case "n":
			p.pendingFile = ""
			p.pendingCmd = ""
			p.agentState = agentWaiting
			p.agent.Messages = append(p.agent.Messages, chatcore.Message{
				Role:    llm.RoleUser,
				Content: "(user skipped the command)",
			})
			p.updateAgentViewport()
			return p, nil
		}
		return p, nil
	}

	switch msg.String() {
	case "esc":
		p.mode = modeManual
		p.agentInput.Blur()
		p.cmdInput.Focus()
		return p, nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		if msg.Alt {
			break
		}
		text := strings.TrimSpace(p.agentInput.Value())
		if text == "" {
			return p, nil
		}
		p.agentInput.Reset()
		p.agentDoneMsg = ""
		p.agentStuckMsg = ""
		p.agent.SendUser(text)
		p.agentState = agentStreaming
		p.updateAgentViewport()
		return p, tea.Batch(p.agent.StartStream(p.buildAgentPrompt()), p.agentSpin.Tick)
	}

	var cmd tea.Cmd
	p.agentInput, cmd = p.agentInput.Update(msg)
	return p, cmd
}

func (p *Pane) checkAgentToolCall() {
	last := p.agent.LastAssistantContent()
	tc := extractToolCall(last)
	if tc == nil {
		p.agentState = agentWaiting
		return
	}

	switch tc.kind {
	case "run_cmd":
		p.pendingCmd = tc.content
		p.pendingFile = ""
		p.agentState = agentPending
	case "read_file":
		p.pendingFile = tc.content
		p.pendingCmd = ""
		p.agentState = agentPending
	case "done":
		p.agentDoneMsg = tc.content
		p.agentState = agentDone
	case "stuck":
		p.agentStuckMsg = tc.content
		p.agentState = agentStuck
	}
	p.updateAgentViewport()
}

func (p *Pane) approveReadFile() tea.Cmd {
	filePath := p.pendingFile
	p.pendingFile = ""

	fullPath := filepath.Join(p.projectRoot, filePath)
	content, err := project.ReadFile(fullPath)
	if err != nil {
		content = fmt.Sprintf("Error reading file: %s", err)
	}

	result := fmt.Sprintf("File: %s\n\n%s", filePath, content)

	p.agent.Messages = append(p.agent.Messages, chatcore.Message{
		Role:    llm.RoleUser,
		Content: result,
	})
	p.agent.Messages = append(p.agent.Messages, chatcore.Message{
		Role:    llm.RoleAssistant,
		Content: "",
	})
	p.agent.Streaming = true
	p.agentState = agentStreaming
	p.updateAgentViewport()
	return tea.Batch(p.agent.StartStream(p.buildAgentPrompt()), p.agentSpin.Tick)
}

func (p *Pane) feedOutputToAgent() tea.Cmd {
	if len(p.runs) == 0 {
		p.agentState = agentWaiting
		return nil
	}
	last := p.runs[len(p.runs)-1]
	result := fmt.Sprintf("Command: %s\nExit code: %d\nOutput:\n%s", last.cmd, last.exitCode, last.output.String())

	// Trim output if too long
	if len(result) > 4000 {
		result = result[:2000] + "\n...(truncated)...\n" + result[len(result)-1500:]
	}

	p.agent.Messages = append(p.agent.Messages, chatcore.Message{
		Role:    llm.RoleUser,
		Content: result,
	})
	p.agent.Messages = append(p.agent.Messages, chatcore.Message{
		Role:    llm.RoleAssistant,
		Content: "",
	})
	p.agent.Streaming = true
	p.agentState = agentStreaming
	p.updateAgentViewport()
	return tea.Batch(p.agent.StartStream(p.buildAgentPrompt()), p.agentSpin.Tick)
}

func (p *Pane) buildAgentPrompt() string {
	var sb strings.Builder
	sb.WriteString(`You are Rigby, a build agent inside rig. You help the user run builds, fix errors, run tests, and execute multi-step workflows.

You have access to the following tools:
- <tool:run_cmd>command</tool:run_cmd> — propose a shell command to run
`)
	if p.projectTree != "" {
		sb.WriteString("- <tool:read_file>path</tool:read_file> — request to read a file's contents\n")
	}
	sb.WriteString(`- <tool:done>message</tool:done> — signal the goal is complete
- <tool:stuck>reason</tool:stuck> — signal you need user input

The user will approve each tool call before it executes. After each command, you'll see the stdout/stderr output and exit code. Analyze the results and decide the next step.

Be concise. Explain what you're doing in 1-2 sentences before each tool call. Only use one tool per response.

`)

	sb.WriteString(chatcore.PanesOverview)
	sb.WriteString("\n")

	if p.projectTree != "" {
		sb.WriteString(fmt.Sprintf("\n## Project Layout\n```\n%s```\n", p.projectTree))
	}

	if p.activePlanTitle != "" {
		sb.WriteString(fmt.Sprintf("\n## Active Plan: %s\n%s\n", p.activePlanTitle, p.activePlanTasks))
	}

	// Include recent command history for context
	if len(p.runs) > 0 {
		sb.WriteString("\n## Recent Commands\n")
		start := len(p.runs) - 3
		if start < 0 {
			start = 0
		}
		for _, r := range p.runs[start:] {
			out := r.output.String()
			if len(out) > 500 {
				out = out[:250] + "\n...\n" + out[len(out)-200:]
			}
			sb.WriteString(fmt.Sprintf("$ %s (exit %d)\n%s\n", r.cmd, r.exitCode, out))
		}
	}

	return sb.String()
}

// --- History ---

func (p *Pane) addToHistory(cmd string) {
	if len(p.history) == 0 || p.history[len(p.history)-1] != cmd {
		p.history = append(p.history, cmd)
		if len(p.history) > 100 {
			p.history = p.history[len(p.history)-100:]
		}
	}
	p.historyIdx = len(p.history)
	p.saveHistory()
}

func historyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rig", "build_history")
}

func (p *Pane) loadHistory() {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, l := range lines {
		if l != "" {
			p.history = append(p.history, l)
		}
	}
	p.historyIdx = len(p.history)
}

func (p *Pane) saveHistory() {
	path := historyPath()
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte(strings.Join(p.history, "\n")+"\n"), 0o644)
}

// --- Views ---

func (p *Pane) View() string {
	if p.width == 0 {
		return ""
	}
	if p.mode == modeAgent {
		return p.viewAgent()
	}
	return p.viewManual()
}

func (p *Pane) viewManual() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  build"))
	if p.activePlanTitle != "" {
		title := p.activePlanTitle
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(dim.Render(fmt.Sprintf("  plan: %s", title)))
	}
	b.WriteString("\n")

	b.WriteString(p.outputVP.View())
	b.WriteString("\n")

	if p.cmdRunning {
		b.WriteString(p.agentSpin.View() + " running...")
		b.WriteString("\n")
	}

	b.WriteString(fmt.Sprintf("  %s", p.cmdInput.View()))
	b.WriteString("\n")

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  enter=run  ctrl+c=kill  ctrl+l=clear  ↑/↓=history  c=agent mode"))

	return b.String()
}

func (p *Pane) viewAgent() string {
	var b strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  build agent"))
	if p.activePlanTitle != "" {
		title := p.activePlanTitle
		if len(title) > 40 {
			title = title[:37] + "..."
		}
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(dim.Render(fmt.Sprintf("  plan: %s", title)))
	}
	b.WriteString("\n")

	b.WriteString(p.agentVP.View())
	b.WriteString("\n")

	if p.agent.Streaming {
		label := " thinking..."
		if last := p.agent.LastAssistantContent(); last == "" {
			label = " waiting for model..."
		}
		b.WriteString(p.agentSpin.View() + label + "\n")
	} else if p.cmdRunning {
		b.WriteString(p.agentSpin.View() + " running command...\n")
	}

	if p.agentState == agentPending {
		if p.pendingFile != "" {
			b.WriteString(p.renderReadFileCard(p.pendingFile))
		} else {
			b.WriteString(p.renderCmdCard(p.pendingCmd))
		}
	} else if p.agentState == agentDone {
		done := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Bold(true)
		b.WriteString(done.Render(fmt.Sprintf("  done: %s", p.agentDoneMsg)))
		b.WriteString("\n")
	} else if p.agentState == agentStuck {
		stuck := lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Bold(true)
		b.WriteString(stuck.Render(fmt.Sprintf("  stuck: %s", p.agentStuckMsg)))
		b.WriteString("\n")
	}

	if p.agentState != agentPending && !p.cmdRunning {
		b.WriteString(p.agentInput.View())
		b.WriteString("\n")
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	ctx := chatcore.ContextStatus(&p.agent)
	if p.agentState == agentPending {
		b.WriteString(help.Render("  y=approve  n=skip") + "  " + ctx)
	} else {
		b.WriteString(help.Render("  enter=send  esc=manual mode") + "  " + ctx)
	}

	return b.String()
}

func (p *Pane) renderCmdCard(cmd string) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3B82F6")).
		Padding(0, 1).
		Width(p.width - 6)

	var sb strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#3B82F6"))
	sb.WriteString(header.Render("run command?"))
	sb.WriteString("\n\n")

	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Bold(true)
	sb.WriteString("  $ " + cmdStyle.Render(cmd))
	sb.WriteString("\n\n")

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	sb.WriteString(hint.Render("y = approve    n = skip"))

	return "  " + border.Render(sb.String()) + "\n"
}

func (p *Pane) renderReadFileCard(path string) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#F59E0B")).
		Padding(0, 1).
		Width(p.width - 6)

	var sb strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F59E0B"))
	sb.WriteString(header.Render("read file?"))
	sb.WriteString("\n\n")

	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB")).Bold(true)
	sb.WriteString("  " + pathStyle.Render(path))
	sb.WriteString("\n\n")

	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	sb.WriteString(hint.Render("y = approve    n = skip"))

	return "  " + border.Render(sb.String()) + "\n"
}

func (p *Pane) updateOutputViewport() {
	promptStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	successStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444"))

	var sb strings.Builder
	for _, r := range p.runs {
		sb.WriteString(promptStyle.Render("$ "+r.cmd) + "\n")
		sb.WriteString(r.output.String())
		if !r.running {
			dur := fmt.Sprintf("%.1fs", r.duration.Seconds())
			if r.exitCode == 0 {
				sb.WriteString(successStyle.Render(fmt.Sprintf("(exit 0, %s)", dur)) + "\n")
			} else {
				sb.WriteString(failStyle.Render(fmt.Sprintf("(exit %d, %s)", r.exitCode, dur)) + "\n")
			}
		}
		sb.WriteString("\n")
	}
	p.outputVP.SetContent(sb.String())
	p.outputVP.GotoBottom()
}

func (p *Pane) updateAgentViewport() {
	var sb strings.Builder
	for _, m := range p.agent.Messages {
		switch m.Role {
		case llm.RoleUser:
			// Skip raw command output feedback (starts with "Command:")
			if strings.HasPrefix(m.Content, "Command:") {
				dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
				lines := strings.Split(m.Content, "\n")
				if len(lines) > 5 {
					lines = append(lines[:3], "  ...", lines[len(lines)-1])
				}
				sb.WriteString(dim.Render(strings.Join(lines, "\n")) + "\n\n")
			} else {
				sb.WriteString(chatcore.UserStyle.Render("you") + "\n")
				sb.WriteString(m.Content + "\n\n")
			}
		case llm.RoleAssistant:
			sb.WriteString(chatcore.AssistantStyle.Render("rigby") + "\n")
			content := m.Content
			if content == "" && p.agent.Streaming {
				content = "..."
			}
			content = stripToolBlocks(content)
			sb.WriteString(content + "\n\n")
		}
	}
	p.agentVP.SetContent(sb.String())
	p.agentVP.GotoBottom()
}
