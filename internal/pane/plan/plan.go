package plan

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vicontiveros00/rig/internal/chatcore"
	"github.com/vicontiveros00/rig/internal/history"
	"github.com/vicontiveros00/rig/internal/llm"
	"github.com/vicontiveros00/rig/internal/messages"
	"github.com/vicontiveros00/rig/internal/pane"
)

type viewMode int

const (
	viewList viewMode = iota
	viewChat
)

type editMode int

const (
	editNone editMode = iota
	editAddTask
	editTaskTitle
	editTaskNotes
	editPlanTitle
)

type flatEntry struct {
	taskIdx []int
	depth   int
	task    *history.Task
}

type planListLoadedMsg struct {
	metas []history.PlanMeta
}

type planLoadedMsg struct {
	plan history.Plan
	err  error
}

type pendingToolCall struct {
	tasks []history.Task
}

type Pane struct {
	plan    history.Plan
	entries []flatEntry
	cursor  int
	width   int
	height  int

	view   viewMode
	mode   editMode
	input  textinput.Model
	listVP viewport.Model

	// Chat state
	chat        chatcore.Engine
	chatInput   textarea.Model
	chatVP      viewport.Model
	chatSpinner spinner.Model
	applied     string
	pendingTool *pendingToolCall

	pickerOpen  bool
	pickerItems []history.PlanMeta
	pickerIdx   int

	confirmDelete bool
	nextTaskID    int
}

func New(provider llm.Provider, model string) pane.Pane {
	ta := textarea.New()
	ta.Placeholder = "ask Rigby about the plan..."
	ta.ShowLineNumbers = false
	ta.SetHeight(3)
	ta.CharLimit = 0

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	p := &Pane{
		chat:        chatcore.Engine{Provider: provider, Model: model},
		chatInput:   ta,
		chatSpinner: sp,
	}
	if id, _ := history.GetActivePlan(); id != "" {
		if loaded, err := history.LoadPlan(id); err == nil {
			p.plan = loaded
		}
	}
	if p.plan.ID == "" {
		p.plan = newPlan("untitled plan")
	}
	p.rebuildEntries()
	return p
}

func newPlan(title string) history.Plan {
	return history.Plan{
		ID:        history.GeneratePlanID(title),
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func (p *Pane) Name() string      { return "plan" }
func (p *Pane) ShortHelp() string { return "task planning" }

func (p *Pane) SetSize(w, h int) {
	p.width = w
	p.height = h

	// List viewport (header + help = ~4 lines overhead)
	listH := h - 4
	if listH < 1 {
		listH = 1
	}
	p.listVP.Width = w
	p.listVP.Height = listH

	// Chat viewport
	inputH := 3
	p.chatInput.SetWidth(w - 2)
	p.chatInput.SetHeight(inputH)
	vpH := h - inputH - 4
	if vpH < 1 {
		vpH = 1
	}
	p.chatVP.Width = w
	p.chatVP.Height = vpH
}

func (p *Pane) Init() tea.Cmd {
	return tea.Batch(p.emitPlanChanged(), textarea.Blink, p.chatSpinner.Tick)
}

func (p *Pane) genTaskID() string {
	p.nextTaskID++
	return fmt.Sprintf("t%d", p.nextTaskID)
}

func (p *Pane) rebuildEntries() {
	p.entries = nil
	p.flattenTasks(p.plan.Tasks, nil, 0)
}

func (p *Pane) flattenTasks(tasks []history.Task, path []int, depth int) {
	for i := range tasks {
		idx := append(append([]int{}, path...), i)
		p.entries = append(p.entries, flatEntry{
			taskIdx: idx,
			depth:   depth,
			task:    &tasks[i],
		})
		p.flattenTasks(tasks[i].Children, idx, depth+1)
	}
}

func (p *Pane) getTaskSlice(path []int) (*[]history.Task, int) {
	tasks := &p.plan.Tasks
	for i := 0; i < len(path)-1; i++ {
		tasks = &(*tasks)[path[i]].Children
	}
	return tasks, path[len(path)-1]
}

// --- Update ---

func (p *Pane) Update(msg tea.Msg) (pane.Pane, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ModelSelectedMsg:
		p.chat.SetProvider(msg.Provider, msg.Model)
		return p, nil

	case planListLoadedMsg:
		p.pickerItems = msg.metas
		p.pickerIdx = 0
		p.pickerOpen = true
		return p, nil

	case planLoadedMsg:
		if msg.err == nil {
			p.plan = msg.plan
			p.chat.Messages = nil
			_ = history.SetActivePlan(p.plan.ID)
			p.rebuildEntries()
			if p.cursor >= len(p.entries) {
				p.cursor = max(0, len(p.entries)-1)
			}
			return p, p.emitPlanChanged()
		}
		return p, nil

	case chatcore.ChunkMsg:
		done := p.chat.HandleChunk(msg.Chunk)
		p.updateChatViewport()
		if done {
			p.checkForToolBlock()
			return p, nil
		}
		return p, p.chat.WaitForChunk()

	case spinner.TickMsg:
		if p.chat.Streaming {
			var cmd tea.Cmd
			p.chatSpinner, cmd = p.chatSpinner.Update(msg)
			return p, cmd
		}

	case tea.KeyMsg:
		if p.pickerOpen {
			return p.updatePicker(msg)
		}
		if p.view == viewChat {
			return p.updateChat(msg)
		}
		if p.confirmDelete {
			return p.updateConfirmDelete(msg)
		}
		if p.mode != editNone {
			return p.updateEdit(msg)
		}
		return p.updateList(msg)
	}

	return p, nil
}

func (p *Pane) scrollToCursor() {
	if p.cursor < p.listVP.YOffset {
		p.listVP.SetYOffset(p.cursor)
	} else if p.cursor >= p.listVP.YOffset+p.listVP.Height {
		p.listVP.SetYOffset(p.cursor - p.listVP.Height + 1)
	}
}

func (p *Pane) updateList(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if p.cursor > 0 {
			p.cursor--
			p.scrollToCursor()
		}
	case "down", "j":
		if p.cursor < len(p.entries)-1 {
			p.cursor++
			p.scrollToCursor()
		}
	case " ":
		if len(p.entries) > 0 {
			t := p.entries[p.cursor].task
			switch t.Status {
			case "pending":
				t.Status = "in_progress"
			case "in_progress":
				t.Status = "done"
			default:
				t.Status = "pending"
			}
			return p, p.autoSave()
		}
	case "a":
		p.mode = editAddTask
		p.input = textinput.New()
		p.input.Placeholder = "new task title"
		p.input.Focus()
		p.input.Width = p.width - 10
	case "e":
		if len(p.entries) > 0 {
			p.mode = editTaskTitle
			p.input = textinput.New()
			p.input.SetValue(p.entries[p.cursor].task.Title)
			p.input.Focus()
			p.input.Width = p.width - 10
		}
	case "n":
		if len(p.entries) > 0 {
			p.mode = editTaskNotes
			p.input = textinput.New()
			p.input.Placeholder = "notes..."
			p.input.SetValue(p.entries[p.cursor].task.Notes)
			p.input.Focus()
			p.input.Width = p.width - 10
		}
	case "E":
		p.mode = editPlanTitle
		p.input = textinput.New()
		p.input.SetValue(p.plan.Title)
		p.input.Focus()
		p.input.Width = p.width - 10
	case "d":
		if len(p.entries) > 0 {
			p.confirmDelete = true
		}
	case "tab":
		if len(p.entries) > 0 && p.cursor > 0 {
			p.indentTask()
			return p, p.autoSave()
		}
	case "shift+tab":
		if len(p.entries) > 0 {
			p.unindentTask()
			return p, p.autoSave()
		}
	case "c":
		p.view = viewChat
		p.chatInput.Focus()
		p.applied = ""
		p.updateChatViewport()
	case "ctrl+n":
		return p, p.newPlanCmd()
	case "ctrl+o":
		return p, p.openPicker()
	case "ctrl+s":
		return p, p.autoSave()
	}
	return p, nil
}

func (p *Pane) updateChat(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	if p.chat.Streaming {
		if msg.String() == "esc" {
			p.chat.CancelStream()
			return p, nil
		}
		return p, nil
	}

	if p.pendingTool != nil {
		switch msg.String() {
		case "y":
			return p, p.applyPendingTool()
		case "n":
			p.applied = "skipped"
			p.pendingTool = nil
			p.updateChatViewport()
			return p, nil
		}
		return p, nil
	}

	switch msg.String() {
	case "esc":
		p.view = viewList
		p.chatInput.Blur()
		return p, nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		if msg.Alt {
			break
		}
		text := strings.TrimSpace(p.chatInput.Value())
		if text == "" {
			return p, nil
		}
		p.chatInput.Reset()
		p.applied = ""
		p.chat.SendUser(text)
		p.updateChatViewport()
		return p, tea.Batch(p.chat.StartStream(p.planSystemPrompt()), p.chatSpinner.Tick)
	}

	var cmd tea.Cmd
	p.chatInput, cmd = p.chatInput.Update(msg)
	return p, cmd
}

func (p *Pane) updateEdit(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.mode = editNone
		return p, nil
	case "enter":
		val := strings.TrimSpace(p.input.Value())
		switch p.mode {
		case editAddTask:
			if val != "" {
				task := history.Task{
					ID:     p.genTaskID(),
					Title:  val,
					Status: "pending",
				}
				if p.cursor < len(p.entries) {
					entry := p.entries[p.cursor]
					slice, idx := p.getTaskSlice(entry.taskIdx)
					*slice = append((*slice)[:idx+1], append([]history.Task{task}, (*slice)[idx+1:]...)...)
				} else {
					p.plan.Tasks = append(p.plan.Tasks, task)
				}
				p.rebuildEntries()
			}
		case editTaskTitle:
			if val != "" && p.cursor < len(p.entries) {
				p.entries[p.cursor].task.Title = val
			}
		case editTaskNotes:
			if p.cursor < len(p.entries) {
				p.entries[p.cursor].task.Notes = val
			}
		case editPlanTitle:
			if val != "" {
				p.plan.Title = val
			}
		}
		p.mode = editNone
		return p, p.autoSave()
	}

	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

func (p *Pane) updateConfirmDelete(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "y":
		if p.cursor < len(p.entries) {
			entry := p.entries[p.cursor]
			slice, idx := p.getTaskSlice(entry.taskIdx)
			*slice = append((*slice)[:idx], (*slice)[idx+1:]...)
			p.rebuildEntries()
			if p.cursor >= len(p.entries) && p.cursor > 0 {
				p.cursor--
			}
		}
		p.confirmDelete = false
		return p, p.autoSave()
	case "n", "esc":
		p.confirmDelete = false
	}
	return p, nil
}

// --- Chat streaming ---

func (p *Pane) planSystemPrompt() string {
	tasksMD := formatPlanMarkdown(p.plan.Tasks, 0)
	prompt := fmt.Sprintf(`You are Rigby, a planning assistant inside rig. You are in the plan pane helping the user work on their plan.

## Current Plan: %q

%s
When the user asks you to create, expand, or modify tasks, propose changes using a tool block:

<tool:apply_plan>
- [ ] task title
  - [ ] subtask
  notes: optional notes
</tool:apply_plan>

This will prompt the user to confirm before applying. You can include explanatory text before and after the tool block. Use the same markdown task format as the current plan shown above.
Keep responses focused on planning — be concise and action-oriented.`, p.plan.Title, tasksMD)
	return prompt
}

func (p *Pane) checkForToolBlock() {
	lastAssistant := p.chat.LastAssistantContent()

	block := ExtractToolBlock(lastAssistant)
	if block == "" {
		return
	}

	tasks := ParsePlanMarkdown(block)
	if len(tasks) == 0 {
		return
	}

	p.pendingTool = &pendingToolCall{tasks: tasks}
}

func (p *Pane) applyPendingTool() tea.Cmd {
	if p.pendingTool == nil {
		return nil
	}

	tasks := p.pendingTool.tasks
	if len(p.plan.Tasks) == 0 {
		p.plan.Tasks = tasks
	} else {
		p.plan.Tasks = append(p.plan.Tasks, tasks...)
	}
	p.pendingTool = nil
	p.rebuildEntries()
	p.applied = fmt.Sprintf("applied %d tasks", len(tasks))
	p.updateChatViewport()
	return p.autoSave()
}

// --- Task operations ---

func (p *Pane) indentTask() {
	entry := p.entries[p.cursor]
	if entry.depth >= 2 {
		return
	}
	path := entry.taskIdx
	if len(path) == 0 {
		return
	}
	slice, idx := p.getTaskSlice(path)
	if idx == 0 {
		return
	}

	task := (*slice)[idx]
	*slice = append((*slice)[:idx], (*slice)[idx+1:]...)
	parent := &(*slice)[idx-1]
	parent.Children = append(parent.Children, task)
	p.rebuildEntries()
}

func (p *Pane) unindentTask() {
	entry := p.entries[p.cursor]
	if entry.depth == 0 {
		return
	}
	path := entry.taskIdx
	childSlice, childIdx := p.getTaskSlice(path)
	task := (*childSlice)[childIdx]
	*childSlice = append((*childSlice)[:childIdx], (*childSlice)[childIdx+1:]...)

	parentPath := path[:len(path)-1]
	parentSlice, parentIdx := p.getTaskSlice(parentPath)
	*parentSlice = append((*parentSlice)[:parentIdx+1], append([]history.Task{task}, (*parentSlice)[parentIdx+1:]...)...)
	p.rebuildEntries()
}

func (p *Pane) autoSave() tea.Cmd {
	p.plan.UpdatedAt = time.Now()
	_ = history.SavePlan(p.plan)
	_ = history.SetActivePlan(p.plan.ID)
	return p.emitPlanChanged()
}

func (p *Pane) emitPlanChanged() tea.Cmd {
	title := p.plan.Title
	tasksMD := formatPlanMarkdown(p.plan.Tasks, 0)
	return func() tea.Msg {
		return messages.ActivePlanChangedMsg{
			PlanTitle: title,
			PlanTasks: tasksMD,
		}
	}
}

func formatPlanMarkdown(tasks []history.Task, depth int) string {
	var sb strings.Builder
	indent := strings.Repeat("  ", depth)
	for _, t := range tasks {
		marker := "[ ]"
		if t.Status == "in_progress" {
			marker = "[~]"
		} else if t.Status == "done" {
			marker = "[x]"
		}
		sb.WriteString(fmt.Sprintf("%s- %s %s\n", indent, marker, t.Title))
		if t.Notes != "" {
			sb.WriteString(fmt.Sprintf("%s  notes: %s\n", indent, t.Notes))
		}
		if len(t.Children) > 0 {
			sb.WriteString(formatPlanMarkdown(t.Children, depth+1))
		}
	}
	return sb.String()
}

func (p *Pane) newPlanCmd() tea.Cmd {
	p.autoSave()
	p.plan = newPlan("untitled plan")
	p.cursor = 0
	p.chat.Messages = nil
	p.rebuildEntries()
	p.mode = editPlanTitle
	p.input = textinput.New()
	p.input.Placeholder = "plan title"
	p.input.Focus()
	p.input.Width = p.width - 10
	return nil
}

func (p *Pane) openPicker() tea.Cmd {
	return func() tea.Msg {
		metas, _ := history.ListPlans()
		return planListLoadedMsg{metas: metas}
	}
}

func (p *Pane) updatePicker(msg tea.KeyMsg) (pane.Pane, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.pickerOpen = false
		return p, nil
	case "up", "k":
		if p.pickerIdx > 0 {
			p.pickerIdx--
		}
	case "down", "j":
		if p.pickerIdx < len(p.pickerItems)-1 {
			p.pickerIdx++
		}
	case "enter":
		if len(p.pickerItems) > 0 {
			meta := p.pickerItems[p.pickerIdx]
			p.pickerOpen = false
			return p, func() tea.Msg {
				plan, err := history.LoadPlan(meta.ID)
				return planLoadedMsg{plan: plan, err: err}
			}
		}
	}
	return p, nil
}

// --- Views ---

func (p *Pane) View() string {
	if p.width == 0 {
		return ""
	}
	if p.pickerOpen {
		return p.viewPicker()
	}
	if p.view == viewChat {
		return p.viewChat()
	}
	return p.viewPlan()
}

func (p *Pane) updateListViewport() {
	var content strings.Builder

	if len(p.entries) == 0 && p.mode == editNone {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		content.WriteString(dim.Render("  no tasks yet — press 'a' to add one or 'c' to chat with Rigby"))
		content.WriteString("\n")
	}

	for i, entry := range p.entries {
		line := p.renderTask(entry)
		if i == p.cursor {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Width(p.width - 2).
				Render(line)
		}
		content.WriteString(line)
		content.WriteString("\n")
	}

	if p.mode != editNone {
		content.WriteString("\n")
		var label string
		switch p.mode {
		case editAddTask:
			label = "new task:"
		case editTaskTitle:
			label = fmt.Sprintf("task [%s]:", p.entries[p.cursor].task.ID)
		case editTaskNotes:
			label = fmt.Sprintf("notes [%s]:", p.entries[p.cursor].task.Title)
		case editPlanTitle:
			label = "plan name:"
		}
		labelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#9CA3AF"))
		content.WriteString(fmt.Sprintf("  %s %s\n", labelStyle.Render(label), p.input.View()))
	}

	if p.confirmDelete && p.cursor < len(p.entries) {
		content.WriteString("\n")
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("#EF4444")).Bold(true)
		content.WriteString(warn.Render(fmt.Sprintf("  delete %q? (y/n)", p.entries[p.cursor].task.Title)))
		content.WriteString("\n")
	}

	p.listVP.SetContent(content.String())
}

func (p *Pane) viewPlan() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))

	total, done := history.CountTasksPublic(p.plan.Tasks)
	titleLine := fmt.Sprintf("  plan: %s", p.plan.Title)
	countLine := fmt.Sprintf("%d/%d tasks", done, total)
	headerWidth := p.width - lipgloss.Width(titleLine) - lipgloss.Width(countLine) - 4
	if headerWidth < 0 {
		headerWidth = 0
	}
	b.WriteString(headerStyle.Render(titleLine))
	b.WriteString(strings.Repeat(" ", headerWidth))
	b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render(countLine))
	b.WriteString("\n\n")

	p.updateListViewport()
	b.WriteString(p.listVP.View())
	b.WriteString("\n")

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  a=add  e=edit task  space=toggle  d=del  n=notes  c=chat  E=rename  ctrl+n=new  ctrl+o=history"))

	return b.String()
}

func (p *Pane) viewChat() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render(fmt.Sprintf("  plan chat: %s", p.plan.Title)))
	b.WriteString("\n")

	b.WriteString(p.chatVP.View())
	b.WriteString("\n")

	if p.chat.Streaming {
		b.WriteString(p.chatSpinner.View() + " streaming...\n")
	}
	if p.applied != "" {
		applyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))
		b.WriteString(applyStyle.Render("  "+p.applied) + "\n")
	}

	b.WriteString(p.chatInput.View())
	b.WriteString("\n")

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	if p.pendingTool != nil {
		b.WriteString(help.Render("  y=apply  n=skip"))
	} else {
		b.WriteString(help.Render("  enter=send  esc=back"))
	}

	return b.String()
}

func (p *Pane) updateChatViewport() {
	var sb strings.Builder
	for _, m := range p.chat.Messages {
		switch m.Role {
		case llm.RoleUser:
			sb.WriteString(chatcore.UserStyle.Render("you") + "\n")
			sb.WriteString(m.Content + "\n\n")
		case llm.RoleAssistant:
			sb.WriteString(chatcore.AssistantStyle.Render("rigby") + "\n")
			content := m.Content
			if content == "" && p.chat.Streaming {
				content = "..."
			}
			content = renderToolBlocks(content)
			sb.WriteString(content + "\n\n")
		}
	}

	if p.pendingTool != nil {
		sb.WriteString(p.renderToolCard(p.pendingTool.tasks))
	}

	p.chatVP.SetContent(sb.String())
	p.chatVP.GotoBottom()
}

func renderToolBlocks(content string) string {
	const open = "<tool:apply_plan>"
	const close = "</tool:apply_plan>"
	result := content
	for {
		start := strings.Index(result, open)
		if start == -1 {
			break
		}
		endTag := strings.Index(result[start:], close)
		if endTag == -1 {
			break
		}
		end := start + endTag + len(close)
		result = result[:start] + result[end:]
	}
	return result
}

func (p *Pane) renderToolCard(tasks []history.Task) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#8B5CF6")).
		Padding(0, 1).
		Width(p.width - 6)

	var sb strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	sb.WriteString(header.Render("apply to plan?"))
	sb.WriteString("\n\n")

	for _, t := range tasks {
		p.renderToolTask(&sb, t, 0)
	}

	sb.WriteString("\n")
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	sb.WriteString(hint.Render("y = apply    n = skip"))

	return "  " + border.Render(sb.String()) + "\n"
}

func (p *Pane) renderToolTask(sb *strings.Builder, t history.Task, depth int) {
	indent := strings.Repeat("  ", depth)
	icon := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("○")
	if t.Status == "in_progress" {
		icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("●")
	} else if t.Status == "done" {
		icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("✓")
	}
	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	sb.WriteString(fmt.Sprintf("%s%s %s\n", indent, icon, titleStyle.Render(t.Title)))
	if t.Notes != "" {
		noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Italic(true)
		sb.WriteString(fmt.Sprintf("%s  %s\n", indent, noteStyle.Render("notes: "+t.Notes)))
	}
	for _, child := range t.Children {
		p.renderToolTask(sb, child, depth+1)
	}
}

func (p *Pane) renderTask(entry flatEntry) string {
	indent := strings.Repeat("  ", entry.depth)
	t := entry.task

	var icon string
	switch t.Status {
	case "in_progress":
		icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("●")
	case "done":
		icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981")).Render("✓")
	default:
		icon = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Render("○")
	}

	titleStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
	if t.Status == "done" {
		titleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Strikethrough(true)
	}

	line := fmt.Sprintf("  %s%s %s", indent, icon, titleStyle.Render(t.Title))

	if t.Status == "in_progress" {
		line += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("#F59E0B")).Render("in progress")
	}

	if t.Notes != "" {
		noteStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280")).Italic(true)
		line += "\n  " + indent + "  " + noteStyle.Render(t.Notes)
	}

	return line
}

func (p *Pane) viewPicker() string {
	var b strings.Builder
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8B5CF6"))
	b.WriteString(headerStyle.Render("  saved plans"))
	b.WriteString("\n\n")

	if len(p.pickerItems) == 0 {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
		b.WriteString(dim.Render("  no saved plans"))
		b.WriteString("\n")
	}

	for i, meta := range p.pickerItems {
		ts := meta.CreatedAt.Format("2006-01-02 15:04")
		progress := fmt.Sprintf("%d/%d", meta.DoneCount, meta.TaskCount)
		progressStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#10B981"))

		line := fmt.Sprintf("  %s  %s  %s", ts, meta.Title, progressStyle.Render(progress))

		if i == p.pickerIdx {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("#1E1B4B")).
				Width(p.width - 2).
				Render("> " + line[2:])
		}

		b.WriteString(line)
		b.WriteString("\n\n")
	}

	help := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B7280"))
	b.WriteString(help.Render("  ↑/↓ navigate  enter = load  esc = cancel"))

	return b.String()
}

