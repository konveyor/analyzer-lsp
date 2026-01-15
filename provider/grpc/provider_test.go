package grpc

import (
	"reflect"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestConvertTypedSlices(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "string slice conversion",
			input: map[string]interface{}{
				"includedPaths": []string{"/path1", "/path2", "/path3"},
			},
			expected: map[string]interface{}{
				"includedPaths": []interface{}{"/path1", "/path2", "/path3"},
			},
		},
		{
			name: "int slice conversion",
			input: map[string]interface{}{
				"ports": []int{8080, 9090, 3000},
			},
			expected: map[string]interface{}{
				"ports": []interface{}{8080, 9090, 3000},
			},
		},
		{
			name: "bool slice conversion",
			input: map[string]interface{}{
				"flags": []bool{true, false, true},
			},
			expected: map[string]interface{}{
				"flags": []interface{}{true, false, true},
			},
		},
		{
			name: "already interface slice",
			input: map[string]interface{}{
				"mixed": []interface{}{"string", 123, true},
			},
			expected: map[string]interface{}{
				"mixed": []interface{}{"string", 123, true},
			},
		},
		{
			name: "multiple fields with different types",
			input: map[string]interface{}{
				"paths":   []string{"/a", "/b"},
				"enabled": true,
				"count":   42,
				"name":    "test",
			},
			expected: map[string]interface{}{
				"paths":   []interface{}{"/a", "/b"},
				"enabled": true,
				"count":   42,
				"name":    "test",
			},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"config": map[string]interface{}{
					"paths": []string{"/nested1", "/nested2"},
				},
			},
			expected: map[string]interface{}{
				"config": map[string]interface{}{
					"paths": []interface{}{"/nested1", "/nested2"},
				},
			},
		},
		{
			name: "slice of maps",
			input: map[string]interface{}{
				"items": []map[string]interface{}{
					{"name": "item1", "tags": []string{"tag1", "tag2"}},
					{"name": "item2", "tags": []string{"tag3"}},
				},
			},
			expected: map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"name": "item1", "tags": []interface{}{"tag1", "tag2"}},
					map[string]interface{}{"name": "item2", "tags": []interface{}{"tag3"}},
				},
			},
		},
		{
			name: "empty slice",
			input: map[string]interface{}{
				"empty": []string{},
			},
			expected: map[string]interface{}{
				"empty": []interface{}{},
			},
		},
		{
			name: "nil value in map",
			input: map[string]interface{}{
				"nullable": nil,
				"paths":    []string{"/path"},
			},
			expected: map[string]interface{}{
				"nullable": nil,
				"paths":    []interface{}{"/path"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertTypedSlices(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("convertTypedSlices() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: nil,
		},
		{
			name:     "string value",
			input:    "test",
			expected: "test",
		},
		{
			name:     "int value",
			input:    42,
			expected: 42,
		},
		{
			name:     "bool value",
			input:    true,
			expected: true,
		},
		{
			name:     "string slice",
			input:    []string{"a", "b", "c"},
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name:     "int slice",
			input:    []int{1, 2, 3},
			expected: []interface{}{1, 2, 3},
		},
		{
			name:     "interface slice",
			input:    []interface{}{"mixed", 123},
			expected: []interface{}{"mixed", 123},
		},
		{
			name: "nested slice",
			input: [][]string{
				{"a", "b"},
				{"c", "d"},
			},
			expected: []interface{}{
				[]interface{}{"a", "b"},
				[]interface{}{"c", "d"},
			},
		},
		{
			name: "map with typed slices",
			input: map[string]interface{}{
				"items": []string{"x", "y"},
			},
			expected: map[string]interface{}{
				"items": []interface{}{"x", "y"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertValue(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("convertValue() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestStructpbIntegration verifies that converted configs can be successfully marshaled by structpb.NewStruct
func TestStructpbIntegration(t *testing.T) {
	tests := []struct {
		name   string
		input  map[string]interface{}
		wantErr bool
	}{
		{
			name: "typed string slice - should work after conversion",
			input: map[string]interface{}{
				"includedPaths": []string{"/path1", "/path2"},
				"excludedDirs":  []string{"/excluded"},
			},
			wantErr: false,
		},
		{
			name: "mixed types including typed slices",
			input: map[string]interface{}{
				"lspServerPath": "/usr/bin/server",
				"includedPaths": []string{"/src", "/lib"},
				"maxDepth":      10,
				"enabled":       true,
			},
			wantErr: false,
		},
		{
			name: "nested config with typed slices",
			input: map[string]interface{}{
				"server": map[string]interface{}{
					"endpoints": []string{"http://localhost:8080", "http://localhost:9090"},
				},
			},
			wantErr: false,
		},
		{
			name: "real-world provider config",
			input: map[string]interface{}{
				"lspServerPath": "/opt/language-servers/java-lsp",
				"includedPaths": []string{"/app/src/main/java"},
				"excludedDirs":  []string{"/app/target", "/app/.git"},
				"encoding":      "utf-8",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert the config
			converted := convertTypedSlices(tt.input)

			// Try to create a structpb.Struct - this should succeed if conversion worked
			_, err := structpb.NewStruct(converted)
			if (err != nil) != tt.wantErr {
				t.Errorf("structpb.NewStruct() after conversion error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}

// TestStructpbDirectFailure verifies that typed slices fail without conversion (documents the original issue)
func TestStructpbDirectFailure(t *testing.T) {
	// This test documents the original issue - typed slices fail with structpb.NewStruct
	config := map[string]interface{}{
		"includedPaths": []string{"/path1", "/path2"}, // typed slice
	}

	// This should fail without conversion
	_, err := structpb.NewStruct(config)
	if err == nil {
		t.Skip("structpb.NewStruct unexpectedly succeeded with typed slice - library behavior may have changed")
	}

	// But should work after conversion
	converted := convertTypedSlices(config)
	_, err = structpb.NewStruct(converted)
	if err != nil {
		t.Errorf("structpb.NewStruct() failed after conversion: %v", err)
	}
}

// TestRequiresConversion verifies the optimization logic for avoiding unnecessary allocations
func TestRequiresConversion(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{
			name:     "nil value",
			input:    nil,
			expected: false,
		},
		{
			name:     "string primitive",
			input:    "test",
			expected: false,
		},
		{
			name:     "int primitive",
			input:    42,
			expected: false,
		},
		{
			name:     "bool primitive",
			input:    true,
			expected: false,
		},
		{
			name:     "typed string slice",
			input:    []string{"a", "b"},
			expected: true,
		},
		{
			name:     "typed int slice",
			input:    []int{1, 2, 3},
			expected: true,
		},
		{
			name:     "interface slice with only primitives",
			input:    []interface{}{"string", 123, true},
			expected: false,
		},
		{
			name:     "interface slice with nested typed slice",
			input:    []interface{}{"string", []string{"nested"}},
			expected: true,
		},
		{
			name:     "interface slice with nested map",
			input:    []interface{}{map[string]interface{}{"key": "value"}},
			expected: true,
		},
		{
			name:     "map",
			input:    map[string]interface{}{"key": "value"},
			expected: true,
		},
		{
			name:     "empty interface slice",
			input:    []interface{}{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := requiresConversion(tt.input)
			if result != tt.expected {
				t.Errorf("requiresConversion() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestArrayConversion verifies that arrays are properly converted
func TestArrayConversion(t *testing.T) {
	// Test with a non-nil array type
	arr := [2]string{"a", "b"}

	result := convertValue(arr)

	// Should be converted to []interface{}
	slice, ok := result.([]interface{})
	if !ok {
		t.Fatalf("Expected []interface{}, got %T", result)
	}

	if len(slice) != 2 {
		t.Errorf("Expected length 2, got %d", len(slice))
	}

	if slice[0] != "a" || slice[1] != "b" {
		t.Errorf("Expected [a, b], got %v", slice)
	}
}

// TestNilMapSemantics verifies the behavior of nil maps in nested structures
func TestNilMapSemantics(t *testing.T) {
	// This test documents the nil map behavior difference from structpb.NewStruct
	// When a nested map is nil, our implementation returns nil (which becomes null_value:NULL_VALUE)
	// whereas structpb.NewStruct would create an empty struct {}

	var nilMap map[string]interface{}
	config := map[string]interface{}{
		"nested": nilMap,
		"key":    "value",
	}

	converted := convertTypedSlices(config)

	// Our implementation returns nil for nil maps
	if converted["nested"] != nil {
		t.Errorf("Expected nil for nested nil map, got %v", converted["nested"])
	}

	// This can still be marshaled by structpb, but will be null instead of {}
	s, err := structpb.NewStruct(converted)
	if err != nil {
		t.Fatalf("structpb.NewStruct failed: %v", err)
	}

	// The nested field will be null_value instead of empty struct
	if s.Fields["nested"].GetNullValue().String() != "NULL_VALUE" {
		t.Errorf("Expected NULL_VALUE for nil map, got %v", s.Fields["nested"])
	}
}

// TestConversionOptimization verifies that no-op conversions return the same slice
func TestConversionOptimization(t *testing.T) {
	// Create a slice that doesn't need conversion
	original := []interface{}{"string", 123, true, 3.14}

	// Convert it
	result := convertValue(original)

	// Verify it's the same slice (no allocation happened)
	originalPtr := reflect.ValueOf(original).Pointer()
	resultPtr := reflect.ValueOf(result).Pointer()

	if originalPtr != resultPtr {
		t.Error("convertValue() allocated a new slice when conversion wasn't needed")
	}

	// Now test with a slice that needs conversion
	needsConversion := []interface{}{"string", []string{"typed", "slice"}}
	result2 := convertValue(needsConversion)

	// This should be a different slice
	originalPtr2 := reflect.ValueOf(needsConversion).Pointer()
	resultPtr2 := reflect.ValueOf(result2).Pointer()

	if originalPtr2 == resultPtr2 {
		t.Error("convertValue() should have allocated a new slice when conversion was needed")
	}
}
