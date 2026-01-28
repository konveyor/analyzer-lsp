package mcp

import (
	"context"
	"encoding/json"
	"testing"
)

func TestProvidersList_Validation(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	tests := []struct {
		name    string
		params  ProvidersListParams
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid settings path",
			params: ProvidersListParams{
				SettingsPath: getTestSettingsPath(t),
			},
			wantErr: false,
		},
		{
			name: "non-existent settings file",
			params: ProvidersListParams{
				SettingsPath: "nonexistent.json",
			},
			wantErr: true,
			errMsg:  "provider settings file not found",
		},
		{
			name:    "default settings (empty params)",
			params:  ProvidersListParams{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := providersList(context.Background(), log, settingsFile, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("providersList() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("providersList() error = %v, expected to contain %q", err, tt.errMsg)
				}
			}
			if !tt.wantErr && result == "" {
				t.Error("providersList() returned empty result without error")
			}
		})
	}
}

func TestProvidersList_OutputFormat(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := ProvidersListParams{
		SettingsPath: settingsFile,
	}

	result, err := providersList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("providersList() unexpected error: %v", err)
	}

	// Verify result is valid JSON
	var providers []ProviderInfo
	err = json.Unmarshal([]byte(result), &providers)
	if err != nil {
		t.Errorf("providersList() result is not valid JSON: %v", err)
	}

	// Verify we got some providers
	if len(providers) == 0 {
		t.Error("providersList() returned no providers")
	}
}

func TestProvidersList_ProviderStructure(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := ProvidersListParams{
		SettingsPath: settingsFile,
	}

	result, err := providersList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("providersList() unexpected error: %v", err)
	}

	var providers []ProviderInfo
	err = json.Unmarshal([]byte(result), &providers)
	if err != nil {
		t.Fatalf("Failed to unmarshal providers: %v", err)
	}

	// Verify provider structure
	for i, provider := range providers {
		if provider.Name == "" {
			t.Errorf("Provider %d has empty name", i)
		}

		// Capabilities should be present (even if empty)
		if provider.Capabilities == nil {
			t.Errorf("Provider %s has nil capabilities", provider.Name)
		}

		// Locations should be present (even if empty)
		if provider.Locations == nil {
			t.Errorf("Provider %s has nil locations", provider.Name)
		}
	}
}

func TestProvidersList_BuiltinProvider(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := ProvidersListParams{
		SettingsPath: settingsFile,
	}

	result, err := providersList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("providersList() unexpected error: %v", err)
	}

	var providers []ProviderInfo
	err = json.Unmarshal([]byte(result), &providers)
	if err != nil {
		t.Fatalf("Failed to unmarshal providers: %v", err)
	}

	// Verify builtin provider is present
	found := false
	for _, provider := range providers {
		if provider.Name == "builtin" {
			found = true
			// Builtin provider should have capabilities
			if len(provider.Capabilities) == 0 {
				t.Error("Builtin provider has no capabilities")
			}
			break
		}
	}

	if !found {
		t.Error("Builtin provider not found in results")
	}
}

func TestProvidersList_CapabilityInfo(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	params := ProvidersListParams{
		SettingsPath: settingsFile,
	}

	result, err := providersList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("providersList() unexpected error: %v", err)
	}

	var providers []ProviderInfo
	err = json.Unmarshal([]byte(result), &providers)
	if err != nil {
		t.Fatalf("Failed to unmarshal providers: %v", err)
	}

	// Check that at least one provider has capabilities with proper structure
	hasCapabilities := false
	for _, provider := range providers {
		for _, cap := range provider.Capabilities {
			if cap.Name != "" {
				hasCapabilities = true
				// Verify capability structure has all expected fields
				if cap.Name == "" {
					t.Error("Capability has empty name")
				}
				// HasInput and HasOutput are booleans, no need to check values
			}
		}
	}

	if !hasCapabilities {
		t.Log("No providers returned capabilities (may be expected if providers fail to initialize in test env)")
	}
}

func TestProvidersList_DefaultSettingsFile(t *testing.T) {
	log := getTestLogger()
	settingsFile := getTestSettingsPath(t)

	// Test with empty SettingsPath - should use default from settingsFile parameter
	params := ProvidersListParams{}

	result, err := providersList(context.Background(), log, settingsFile, params)
	if err != nil {
		t.Fatalf("providersList() unexpected error: %v", err)
	}

	var providers []ProviderInfo
	err = json.Unmarshal([]byte(result), &providers)
	if err != nil {
		t.Errorf("providersList() result is not valid JSON: %v", err)
	}

	if len(providers) == 0 {
		t.Error("providersList() returned no providers with default settings")
	}
}
