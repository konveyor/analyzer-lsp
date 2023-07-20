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
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					As:   "b",
					From: "a",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					As:   "b",
					From: "a",
				},
			},
		}, {
			title: "incorrectly sorted conditions should be sorted",
			entries: []ConditionEntry{
				ConditionEntry{
					As:   "b",
					From: "a",
				},
				ConditionEntry{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					As:   "b",
					From: "a",
				},
			},
		}, {
			title: "incorrectly sorted conditions that branch should be sorted",
			entries: []ConditionEntry{
				ConditionEntry{
					As:   "b",
					From: "a",
				},
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					As:   "c",
					From: "b",
				},
				ConditionEntry{
					As:   "e",
					From: "d",
				},
				ConditionEntry{
					As:   "d",
					From: "b",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					As:   "b",
					From: "a",
				},
				ConditionEntry{
					As:   "c",
					From: "b",
				},
				ConditionEntry{
					As:   "d",
					From: "b",
				},
				ConditionEntry{
					As:   "e",
					From: "d",
				},
			},
		}, {
			title: "longer chains should sort properly",
			entries: []ConditionEntry{
				ConditionEntry{
					From: "e",
					As:   "f",
				},
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					From: "d",
					As:   "e",
				},
				ConditionEntry{
					From: "a",
					As:   "b",
				},
				ConditionEntry{
					From: "b",
					As:   "c",
				},
				ConditionEntry{
					From: "c",
					As:   "d",
				},
				ConditionEntry{
					From: "f",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					From: "a",
					As:   "b",
				},
				ConditionEntry{
					From: "b",
					As:   "c",
				},
				ConditionEntry{
					From: "c",
					As:   "d",
				},
				ConditionEntry{
					From: "d",
					As:   "e",
				},
				ConditionEntry{
					From: "e",
					As:   "f",
				},
				ConditionEntry{
					From: "f",
				},
			},
		}, {
			title: "completely reversed chains should sort properly",
			entries: []ConditionEntry{
				ConditionEntry{
					From: "c",
				},
				ConditionEntry{
					From: "b",
					As:   "c",
				},
				ConditionEntry{
					From: "a",
					As:   "b",
				},
				ConditionEntry{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					From: "a",
					As:   "b",
				},
				ConditionEntry{
					From: "b",
					As:   "c",
				},
				ConditionEntry{
					From: "c",
				},
			},
		}, {
			title: "unused As should not cause error",
			entries: []ConditionEntry{
				ConditionEntry{
					From: "c",
					As:   "d",
				},
				ConditionEntry{
					From: "b",
					As:   "c",
				},
				ConditionEntry{
					From: "a",
					As:   "b",
				},
				ConditionEntry{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
				ConditionEntry{
					From: "a",
					As:   "b",
				},
				ConditionEntry{
					From: "b",
					As:   "c",
				},
				ConditionEntry{
					From: "c",
					As:   "d",
				},
			},
		}, {
			title: "length 1 lists should not cause error",
			entries: []ConditionEntry{
				ConditionEntry{
					As: "a",
				},
			},
			expected: []ConditionEntry{
				ConditionEntry{
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
