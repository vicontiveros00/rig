package build

import "strings"

type toolCall struct {
	kind    string // "run_cmd", "done", "stuck"
	content string
}

func extractToolCall(content string) *toolCall {
	tools := []string{"run_cmd", "read_file", "done", "stuck"}
	for _, name := range tools {
		open := "<tool:" + name + ">"
		close := "</tool:" + name + ">"

		start := strings.LastIndex(content, open)
		if start == -1 {
			continue
		}
		bodyStart := start + len(open)
		endIdx := strings.Index(content[bodyStart:], close)
		if endIdx == -1 {
			continue
		}
		return &toolCall{
			kind:    name,
			content: strings.TrimSpace(content[bodyStart : bodyStart+endIdx]),
		}
	}
	return nil
}

func stripToolBlocks(content string) string {
	tools := []string{"run_cmd", "read_file", "done", "stuck"}
	result := content
	for _, name := range tools {
		open := "<tool:" + name + ">"
		close := "</tool:" + name + ">"
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
	}
	return result
}
