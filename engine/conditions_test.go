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
