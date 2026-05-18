package plan

import (
	"testing"
)

func TestParsePlanMarkdown(t *testing.T) {
	input := `- [ ] first task
- [~] second task (in progress)
- [x] third task (done)
  - [ ] subtask a
  - [x] subtask b
    notes: some notes here
`

	tasks := ParsePlanMarkdown(input)

	if len(tasks) != 3 {
		t.Fatalf("expected 3 top-level tasks, got %d", len(tasks))
	}

	if tasks[0].Status != "pending" {
		t.Errorf("task 1: expected pending, got %s", tasks[0].Status)
	}
	if tasks[0].Title != "first task" {
		t.Errorf("task 1: expected 'first task', got %q", tasks[0].Title)
	}

	if tasks[1].Status != "in_progress" {
		t.Errorf("task 2: expected in_progress, got %s", tasks[1].Status)
	}

	if tasks[2].Status != "done" {
		t.Errorf("task 3: expected done, got %s", tasks[2].Status)
	}
	if len(tasks[2].Children) != 2 {
		t.Fatalf("task 3: expected 2 children, got %d", len(tasks[2].Children))
	}
	if tasks[2].Children[1].Notes != "some notes here" {
		t.Errorf("subtask b: expected notes='some notes here', got %q", tasks[2].Children[1].Notes)
	}
}

func TestParsePlanMarkdownEmpty(t *testing.T) {
	tasks := ParsePlanMarkdown("")
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks for empty input, got %d", len(tasks))
	}
}

func TestParsePlanMarkdownIgnoresNonTaskLines(t *testing.T) {
	input := `some text
- [ ] actual task
more text
# heading
- [ ] another task`

	tasks := ParsePlanMarkdown(input)
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestExtractPlanToolCall(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantKind string
		wantContent string
	}{
		{
			name:    "apply_plan",
			content: "here is my proposal\n<tool:apply_plan>\n- [ ] task one\n- [ ] task two\n</tool:apply_plan>\ndone",
			wantKind: "apply_plan",
			wantContent: "- [ ] task one\n- [ ] task two",
		},
		{
			name:    "read_file",
			content: "let me check\n<tool:read_file>src/main.go</tool:read_file>",
			wantKind: "read_file",
			wantContent: "src/main.go",
		},
		{
			name:    "no tool",
			content: "just regular text",
			wantKind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := ExtractPlanToolCall(tt.content)
			if tt.wantKind == "" {
				if tc != nil {
					t.Errorf("expected nil, got %+v", tc)
				}
				return
			}
			if tc == nil {
				t.Fatal("expected tool call, got nil")
			}
			if tc.kind != tt.wantKind {
				t.Errorf("expected kind=%s, got %s", tt.wantKind, tc.kind)
			}
			if tc.content != tt.wantContent {
				t.Errorf("expected content=%q, got %q", tt.wantContent, tc.content)
			}
		})
	}
}

func TestStripPlanToolBlocks(t *testing.T) {
	input := "before <tool:apply_plan>content</tool:apply_plan> after"
	result := StripPlanToolBlocks(input)
	if result != "before  after" {
		t.Errorf("expected 'before  after', got %q", result)
	}
}

func TestExtractLastPlanBlock(t *testing.T) {
	input := "text\n```plan\n- [ ] task\n```\nmore text"
	result := ExtractLastPlanBlock(input)
	if result != "- [ ] task\n" {
		t.Errorf("expected '- [ ] task\\n', got %q", result)
	}
}

func TestExtractLastPlanBlockNotFound(t *testing.T) {
	result := ExtractLastPlanBlock("no plan block here")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}
