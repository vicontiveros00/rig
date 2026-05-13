package plan

import (
	"fmt"
	"strings"

	"github.com/vicontiveros00/rig/internal/history"
)

var parseTaskID int

func ParsePlanMarkdown(input string) []history.Task {
	lines := strings.Split(input, "\n")
	var root []history.Task
	// Stack tracks parent task slices at each depth
	type level struct {
		depth int
		tasks *[]history.Task
	}
	stack := []level{{depth: -1, tasks: &root}}

	var lastTask *history.Task

	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if trimmed == "" {
			continue
		}

		indent := 0
		for _, ch := range line {
			if ch == ' ' {
				indent++
			} else {
				break
			}
		}

		stripped := strings.TrimSpace(trimmed)

		// Check for notes line
		if strings.HasPrefix(stripped, "notes:") && lastTask != nil {
			lastTask.Notes = strings.TrimSpace(strings.TrimPrefix(stripped, "notes:"))
			continue
		}

		// Check for task line: - [ ] or - [x] or - [~]
		if !strings.HasPrefix(stripped, "- [") {
			continue
		}

		status := "pending"
		rest := ""
		if strings.HasPrefix(stripped, "- [ ] ") {
			status = "pending"
			rest = stripped[6:]
		} else if strings.HasPrefix(stripped, "- [x] ") {
			status = "done"
			rest = stripped[6:]
		} else if strings.HasPrefix(stripped, "- [~] ") {
			status = "in_progress"
			rest = stripped[6:]
		} else {
			continue
		}

		depth := indent / 2

		parseTaskID++
		task := history.Task{
			ID:     fmt.Sprintf("p%d", parseTaskID),
			Title:  strings.TrimSpace(rest),
			Status: status,
		}

		// Pop stack to find parent at correct depth
		for len(stack) > 1 && stack[len(stack)-1].depth >= depth {
			stack = stack[:len(stack)-1]
		}

		parent := stack[len(stack)-1].tasks
		*parent = append(*parent, task)
		lastTask = &(*parent)[len(*parent)-1]

		stack = append(stack, level{depth: depth, tasks: &lastTask.Children})
	}

	return root
}

func ExtractToolBlock(content string) string {
	const open = "<tool:apply_plan>"
	const close = "</tool:apply_plan>"

	start := strings.LastIndex(content, open)
	if start == -1 {
		return ""
	}
	bodyStart := start + len(open)
	endIdx := strings.Index(content[bodyStart:], close)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(content[bodyStart : bodyStart+endIdx])
}

func ExtractLastPlanBlock(content string) string {
	const open = "```plan"
	const close = "```"

	lastStart := -1
	idx := 0
	for {
		start := strings.Index(content[idx:], open)
		if start == -1 {
			break
		}
		lastStart = idx + start
		idx = lastStart + len(open)
	}

	if lastStart == -1 {
		return ""
	}

	bodyStart := lastStart + len(open)
	// Skip to next line
	if nl := strings.IndexByte(content[bodyStart:], '\n'); nl >= 0 {
		bodyStart += nl + 1
	}

	endIdx := strings.Index(content[bodyStart:], close)
	if endIdx == -1 {
		return ""
	}

	return content[bodyStart : bodyStart+endIdx]
}
