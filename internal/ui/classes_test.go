package ui

import (
	"testing"
)

func TestCx(t *testing.T) {
	tests := []struct {
		name  string
		parts []string
		want  string
	}{
		// Happy path
		{name: "One string", parts: []string{"One!"}, want: "One!"},
		{name: "Two strings", parts: []string{"One!", "Two!"}, want: "One! Two!"},
		// Edge cases — degenerate inputs
		{name: "no arguments", parts: nil, want: ""},
		{name: "empty slice", parts: []string{}, want: ""},
		{name: "single empty string", parts: []string{""}, want: ""},
		{name: "all empty", parts: []string{"", "", ""}, want: ""},
		{name: "trailing empty", parts: []string{"a", "b", ""}, want: "a b"},
		{name: "leading empty", parts: []string{"", "a"}, want: "a"},
		{name: "interior empty", parts: []string{"a", "", "b"}, want: "a  b"},
		{name: "part with internal spaces", parts: []string{"px-2 py-1", "rounded"}, want: "px-2 py-1 rounded"},
		{name: "part with surrounding whitespace", parts: []string{"  a  ", "b"}, want: "a   b"},
		// Contract: purely textual join, no dedup or conflict resolution
		{name: "no dedup", parts: []string{"px-2", "px-2"}, want: "px-2 px-2"},
		{name: "no conflict resolution", parts: []string{"px-2", "px-4"}, want: "px-2 px-4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cx(tt.parts...)

			if got != tt.want {
				t.Errorf("Wanted %q, got %q, with input %#v", tt.want, got, tt.parts)
			}
		})
	}
}
