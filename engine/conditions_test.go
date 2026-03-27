package engine

import (
	"reflect"
	"testing"
)

func Test_sortConditionEntries(t *testing.T) {
	tests := []struct {
		title       string
		entries     []ConditionEntry
		expected    []ConditionEntry
		shouldError bool
	}{
		{
			title: "correctly sorted conditions should stay sorted",
			entries: []ConditionEntry{
				{
					As: "a",
				},
				{
					As:   "b",
					From: "a",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
				{
					As:   "b",
					From: "a",
				},
			},
		}, {
			title: "incorrectly sorted conditions should be sorted",
			entries: []ConditionEntry{
				{
					As:   "b",
					From: "a",
				},
				{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
				{
					As:   "b",
					From: "a",
				},
			},
		}, {
			title: "incorrectly sorted conditions that branch should be sorted",
			entries: []ConditionEntry{
				{
					As:   "b",
					From: "a",
				},
				{
					As: "a",
				},
				{
					As:   "c",
					From: "b",
				},
				{
					As:   "e",
					From: "d",
				},
				{
					As:   "d",
					From: "b",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
				{
					As:   "b",
					From: "a",
				},
				{
					As:   "c",
					From: "b",
				},
				{
					As:   "d",
					From: "b",
				},
				{
					As:   "e",
					From: "d",
				},
			},
		}, {
			title: "longer chains should sort properly",
			entries: []ConditionEntry{
				{
					From: "e",
					As:   "f",
				},
				{
					As: "a",
				},
				{
					From: "d",
					As:   "e",
				},
				{
					From: "a",
					As:   "b",
				},
				{
					From: "b",
					As:   "c",
				},
				{
					From: "c",
					As:   "d",
				},
				{
					From: "f",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
				{
					From: "a",
					As:   "b",
				},
				{
					From: "b",
					As:   "c",
				},
				{
					From: "c",
					As:   "d",
				},
				{
					From: "d",
					As:   "e",
				},
				{
					From: "e",
					As:   "f",
				},
				{
					From: "f",
				},
			},
		}, {
			title: "completely reversed chains should sort properly",
			entries: []ConditionEntry{
				{
					From: "c",
				},
				{
					From: "b",
					As:   "c",
				},
				{
					From: "a",
					As:   "b",
				},
				{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
				{
					From: "a",
					As:   "b",
				},
				{
					From: "b",
					As:   "c",
				},
				{
					From: "c",
				},
			},
		}, {
			title: "unused As should not cause error",
			entries: []ConditionEntry{
				{
					From: "c",
					As:   "d",
				},
				{
					From: "b",
					As:   "c",
				},
				{
					From: "a",
					As:   "b",
				},
				{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
				{
					From: "a",
					As:   "b",
				},
				{
					From: "b",
					As:   "c",
				},
				{
					From: "c",
					As:   "d",
				},
			},
		}, {
			title: "length 1 lists should not cause error",
			entries: []ConditionEntry{
				{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				{
					As: "a",
				},
			},
		}}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			sorted := sortConditionEntries(tt.entries)

			if !reflect.DeepEqual(sorted, tt.expected) {
				t.Errorf("expected '%+v', got '%+v'", tt.expected, sorted)
				return
			}
		})
	}
}

func TestChainTemplate_ToMap_YAMLFlowStrings(t *testing.T) {
	tests := []struct {
		name     string
		template ChainTemplate
		want     map[string]string // filepaths and excludedPaths as strings for assertion
	}{
		{
			name:     "empty filepaths and excludedPaths",
			template: ChainTemplate{},
			want: map[string]string{
				"filepaths":     "[]",
				"excludedPaths": "[]",
			},
		},
		{
			name: "simple paths without special chars",
			template: ChainTemplate{
				Filepaths: []string{"pom.xml", "subdir/pom.xml"},
			},
			want: map[string]string{
				"filepaths":     "[pom.xml, subdir/pom.xml]",
				"excludedPaths": "[]",
			},
		},
		{
			name: "paths with spaces and brackets (Content_Types)",
			template: ChainTemplate{
				Filepaths: []string{"/path/to/[Content_Types].xml", "path/with spaces/file.xml"},
			},
			want: map[string]string{
				"filepaths":     "['/path/to/[Content_Types].xml', 'path/with spaces/file.xml']",
				"excludedPaths": "[]",
			},
		},
		{
			name: "excludedPaths with trailing slashes",
			template: ChainTemplate{
				Filepaths:     []string{"pom.xml"},
				ExcludedPaths: []string{"target/", "build/"},
			},
			want: map[string]string{
				"filepaths":     "[pom.xml]",
				"excludedPaths": "[target/, build/]",
			},
		},
		{
			name: "path with single quote",
			template: ChainTemplate{
				Filepaths: []string{"path/with'quote/file.xml"},
			},
			want: map[string]string{
				"filepaths": "['path/with''quote/file.xml']",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.template.ToMap()
			for key, wantStr := range tt.want {
				got, ok := m[key].(string)
				if !ok {
					t.Errorf("ToMap()[%q] = %T, want string", key, m[key])
					continue
				}
				if got != wantStr {
					t.Errorf("ToMap()[%q] = %q, want %q", key, got, wantStr)
				}
			}
		})
	}
}
