package grpc

import (
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/konveyor/analyzer-lsp/provider"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	reflectionpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/types/known/structpb"
)

func TestSubprocessName(t *testing.T) {
	tests := []struct {
		name   string
		config provider.Config
		want   string
	}{
		{
			name: "lspServerName wins over provider name",
			config: provider.Config{
				Name: "go",
				InitConfig: []provider.InitConfig{{
					ProviderSpecificConfig: map[string]interface{}{
						"lspServerName": "generic",
					},
				}},
			},
			want: "generic",
		},
		{
			name: "pylsp for python block",
			config: provider.Config{
				Name: "python",
				InitConfig: []provider.InitConfig{{
					ProviderSpecificConfig: map[string]interface{}{
						"lspServerName": "pylsp",
					},
				}},
			},
			want: "pylsp",
		},
		{
			name: "provider name when lspServerName absent",
			config: provider.Config{
				Name: "yaml",
				InitConfig: []provider.InitConfig{{
					ProviderSpecificConfig: map[string]interface{}{
						"lspServerPath": "/bin/yq",
					},
				}},
			},
			want: "yaml",
		},
		{
			name: "empty lspServerName falls through to provider name",
			config: provider.Config{
				Name: "java",
				InitConfig: []provider.InitConfig{{
					ProviderSpecificConfig: map[string]interface{}{
						"lspServerName": "",
					},
				}},
			},
			want: "java",
		},
		{
			name:   "default generic",
			config: provider.Config{},
			want:   "generic",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subprocessName(tt.config); got != tt.want {
				t.Errorf("subprocessName() = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestWaitForServices(t *testing.T) {
	log := logr.Discard()

	t.Run("gives up when the provider never lists a service", func(t *testing.T) {
		calls := 0
		list := func() ([]string, error) {
			calls++
			return nil, errors.New("connection refused")
		}
		done := make(chan error, 1)
		go func() {
			_, err := waitForServices(list, 50*time.Millisecond, 5*time.Millisecond, log)
			done <- err
		}()
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected an error after the timeout, got nil")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("waitForServices did not return after its timeout")
		}
		if calls < 2 {
			t.Fatalf("expected the listing to be retried, got %d calls", calls)
		}
	})

	t.Run("gives up when the provider lists no services", func(t *testing.T) {
		list := func() ([]string, error) { return nil, nil }
		_, err := waitForServices(list, 50*time.Millisecond, 5*time.Millisecond, log)
		if err == nil {
			t.Fatal("expected an error after the timeout, got nil")
		}
	})

	t.Run("returns the services once the provider is up", func(t *testing.T) {
		calls := 0
		list := func() ([]string, error) {
			calls++
			switch calls {
			case 1:
				return nil, errors.New("connection refused")
			case 2:
				return nil, nil
			}
			return []string{"provider.ProviderService"}, nil
		}
		got, err := waitForServices(list, 5*time.Second, 5*time.Millisecond, log)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !reflect.DeepEqual(got, []string{"provider.ProviderService"}) {
			t.Fatalf("unexpected services: %v", got)
		}
	})
}

func TestCheckServicesRunningGivesUpOnSilentProvider(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Accept every stream, including the reflection one, and never answer.
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		<-stream.Context().Done()
		return stream.Context().Err()
	}))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	oldTimeout, oldInterval := servicesCheckTimeout, servicesCheckInterval
	servicesCheckTimeout, servicesCheckInterval = 200*time.Millisecond, 10*time.Millisecond
	defer func() { servicesCheckTimeout, servicesCheckInterval = oldTimeout, oldInterval }()

	done := make(chan error, 1)
	go func() {
		_, err := checkServicesRunning(conn, logr.Discard())
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from a provider that never answers, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("checkServicesRunning did not return after its timeout")
	}
}

func TestCheckServicesRunningReturnsOnceServicesAreListed(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	// Answer every reflection request with one service, then keep the stream
	// open after the client closes its side instead of ending it.
	srv := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		for {
			var req reflectionpb.ServerReflectionRequest
			if err := stream.RecvMsg(&req); err != nil {
				<-stream.Context().Done()
				return stream.Context().Err()
			}
			resp := &reflectionpb.ServerReflectionResponse{
				MessageResponse: &reflectionpb.ServerReflectionResponse_ListServicesResponse{
					ListServicesResponse: &reflectionpb.ListServiceResponse{
						Service: []*reflectionpb.ServiceResponse{{Name: "provider.ProviderService"}},
					},
				},
			}
			if err := stream.SendMsg(resp); err != nil {
				return err
			}
		}
	}))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	oldTimeout, oldInterval := servicesCheckTimeout, servicesCheckInterval
	servicesCheckTimeout, servicesCheckInterval = 3*time.Second, 10*time.Millisecond
	defer func() { servicesCheckTimeout, servicesCheckInterval = oldTimeout, oldInterval }()

	start := time.Now()
	services, err := checkServicesRunning(conn, logr.Discard())
	if err != nil {
		t.Fatalf("checkServicesRunning() error = %v", err)
	}
	if !reflect.DeepEqual(services, []string{"provider.ProviderService"}) {
		t.Fatalf("checkServicesRunning() = %v", services)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("checkServicesRunning took %v after the services were listed, want it to return right away", elapsed)
	}
}
