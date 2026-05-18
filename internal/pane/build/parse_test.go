package build

import (
	"testing"
)

func TestExtractToolCall(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantKind    string
		wantContent string
	}{
		{
			name:        "run_cmd",
			content:     "I'll run the tests\n<tool:run_cmd>go test ./...</tool:run_cmd>",
			wantKind:    "run_cmd",
			wantContent: "go test ./...",
		},
		{
			name:        "read_file",
			content:     "Let me check that file\n<tool:read_file>internal/config/config.go</tool:read_file>",
			wantKind:    "read_file",
			wantContent: "internal/config/config.go",
		},
		{
			name:        "done",
			content:     "All tests pass!\n<tool:done>build successful, all tests green</tool:done>",
			wantKind:    "done",
			wantContent: "build successful, all tests green",
		},
		{
			name:        "stuck",
			content:     "I can't proceed\n<tool:stuck>need database credentials</tool:stuck>",
			wantKind:    "stuck",
			wantContent: "need database credentials",
		},
		{
			name:    "no tool",
			content: "just some explanation without a tool call",
		},
		{
			name:    "incomplete tag",
			content: "<tool:run_cmd>go build",
		},
		{
			name:        "multiple tools takes last",
			content:     "<tool:run_cmd>first</tool:run_cmd>\n<tool:run_cmd>second</tool:run_cmd>",
			wantKind:    "run_cmd",
			wantContent: "second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tc := extractToolCall(tt.content)
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

func TestStripToolBlocks(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single block",
			input: "before <tool:run_cmd>go build</tool:run_cmd> after",
			want:  "before  after",
		},
		{
			name:  "multiple blocks",
			input: "a <tool:run_cmd>cmd1</tool:run_cmd> b <tool:done>msg</tool:done> c",
			want:  "a  b  c",
		},
		{
			name:  "no blocks",
			input: "plain text",
			want:  "plain text",
		},
		{
			name:  "read_file block",
			input: "checking <tool:read_file>main.go</tool:read_file> now",
			want:  "checking  now",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripToolBlocks(tt.input)
			if got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
