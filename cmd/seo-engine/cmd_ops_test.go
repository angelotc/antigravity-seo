package main

import (
	"testing"
)

func TestExtractHookFilePath(t *testing.T) {
	tests := []struct {
		name         string
		toolInput    map[string]interface{}
		toolResponse map[string]interface{}
		expected     string
	}{
		{
			name:      "TargetFile in input",
			toolInput: map[string]interface{}{"TargetFile": "/path/to/file.html"},
			expected:  "/path/to/file.html",
		},
		{
			name:      "file_path in input",
			toolInput: map[string]interface{}{"file_path": "/path/to/file.html"},
			expected:  "/path/to/file.html",
		},
		{
			name:      "AbsolutePath in input",
			toolInput: map[string]interface{}{"AbsolutePath": "/path/to/file.html"},
			expected:  "/path/to/file.html",
		},
		{
			name:         "target_file in response",
			toolInput:    map[string]interface{}{},
			toolResponse: map[string]interface{}{"target_file": "/path/to/file.html"},
			expected:     "/path/to/file.html",
		},
		{
			name:      "case-insensitive targetfile",
			toolInput: map[string]interface{}{"targetfile": "/path/to/file.html"},
			expected:  "/path/to/file.html",
		},
		{
			name:      "empty map",
			toolInput: map[string]interface{}{},
			expected:  "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractHookFilePath(tc.toolInput, tc.toolResponse)
			if got != tc.expected {
				t.Errorf("extractHookFilePath() = %q, want %q", got, tc.expected)
			}
		})
	}
}
