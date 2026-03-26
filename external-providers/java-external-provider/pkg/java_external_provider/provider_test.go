package java

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
)

func TestBuildMavenProxyEntries(t *testing.T) {
	tests := []struct {
		name      string
		proxy     *provider.Proxy
		wantCount int
		checks    func(t *testing.T, entries []mavenProxyEntry)
	}{
		{
			name:      "nil proxy returns nil",
			proxy:     nil,
			wantCount: 0,
		},
		{
			name:      "empty proxy returns nil",
			proxy:     &provider.Proxy{},
			wantCount: 0,
		},
		{
			name: "http proxy only",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
			},
			wantCount: 1,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				e := entries[0]
				if e.Protocol != "http" {
					t.Errorf("expected protocol http, got %s", e.Protocol)
				}
				if e.Host != "proxy.example.com" {
					t.Errorf("expected host proxy.example.com, got %s", e.Host)
				}
				if e.Port != "3128" {
					t.Errorf("expected port 3128, got %s", e.Port)
				}
				if e.Active != "true" {
					t.Errorf("expected active true, got %s", e.Active)
				}
			},
		},
		{
			name: "https proxy only",
			proxy: &provider.Proxy{
				HTTPSProxy: "http://proxy.example.com:3129",
			},
			wantCount: 1,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				if entries[0].Protocol != "https" {
					t.Errorf("expected protocol https, got %s", entries[0].Protocol)
				}
				if entries[0].Port != "3129" {
					t.Errorf("expected port 3129, got %s", entries[0].Port)
				}
			},
		},
		{
			name: "both http and https proxies",
			proxy: &provider.Proxy{
				HTTPProxy:  "http://proxy.example.com:3128",
				HTTPSProxy: "http://proxy.example.com:3129",
			},
			wantCount: 2,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				if entries[0].ID != "http-proxy-1" {
					t.Errorf("expected id http-proxy-1, got %s", entries[0].ID)
				}
				if entries[1].ID != "https-proxy-2" {
					t.Errorf("expected id https-proxy-2, got %s", entries[1].ID)
				}
			},
		},
		{
			name: "proxy with authentication",
			proxy: &provider.Proxy{
				HTTPProxy: "http://user:pass@proxy.example.com:3128",
			},
			wantCount: 1,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				if entries[0].Username != "user" {
					t.Errorf("expected username user, got %s", entries[0].Username)
				}
				if entries[0].Password != "pass" {
					t.Errorf("expected password pass, got %s", entries[0].Password)
				}
			},
		},
		{
			name: "noProxy converts comma to pipe",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
				NoProxy:   "localhost,127.0.0.1,.example.com",
			},
			wantCount: 1,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				if entries[0].NonProxyHosts != "localhost|127.0.0.1|.example.com" {
					t.Errorf("expected nonProxyHosts with pipes, got %s", entries[0].NonProxyHosts)
				}
			},
		},
		{
			name: "default port for http",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com",
			},
			wantCount: 1,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				if entries[0].Port != "80" {
					t.Errorf("expected default port 80, got %s", entries[0].Port)
				}
			},
		},
		{
			name: "default port for https scheme",
			proxy: &provider.Proxy{
				HTTPSProxy: "https://proxy.example.com",
			},
			wantCount: 1,
			checks: func(t *testing.T, entries []mavenProxyEntry) {
				if entries[0].Port != "443" {
					t.Errorf("expected default port 443, got %s", entries[0].Port)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := buildMavenProxyEntries(tt.proxy)
			if len(entries) != tt.wantCount {
				t.Fatalf("expected %d entries, got %d", tt.wantCount, len(entries))
			}
			if tt.checks != nil {
				tt.checks(t, entries)
			}
		})
	}
}

func TestBuildMavenProxyEntryStruct(t *testing.T) {
	tests := []struct {
		name     string
		proxyURL string
		protocol string
		id       int
		noProxy  string
		wantOK   bool
	}{
		{
			name:     "invalid URL returns false",
			proxyURL: "://bad",
			protocol: "http",
			id:       1,
			wantOK:   false,
		},
		{
			name:     "empty URL returns false",
			proxyURL: "",
			protocol: "http",
			id:       1,
			wantOK:   false,
		},
		{
			name:     "bare hostname without scheme returns false",
			proxyURL: "not-a-url",
			protocol: "http",
			id:       1,
			wantOK:   false,
		},
		{
			name:     "valid proxy URL",
			proxyURL: "http://proxy.example.com:3128",
			protocol: "http",
			id:       1,
			noProxy:  "localhost,127.0.0.1",
			wantOK:   true,
		},
		{
			name:     "proxy URL with credentials",
			proxyURL: "http://user:pass@proxy.example.com:8080",
			protocol: "https",
			id:       2,
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, ok := buildMavenProxyEntryStruct(tt.proxyURL, tt.protocol, tt.id, tt.noProxy)
			if ok != tt.wantOK {
				t.Fatalf("expected ok=%v, got %v", tt.wantOK, ok)
			}
			if !ok {
				return
			}
			if entry.Protocol != tt.protocol {
				t.Errorf("expected protocol %s, got %s", tt.protocol, entry.Protocol)
			}
		})
	}
}

func TestParseMavenSettings(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		checks  func(t *testing.T, s *mavenSettings)
	}{
		{
			name:    "valid minimal settings",
			input:   `<settings><localRepository>/tmp/repo</localRepository></settings>`,
			wantErr: false,
			checks: func(t *testing.T, s *mavenSettings) {
				if s.LocalRepository != "/tmp/repo" {
					t.Errorf("expected /tmp/repo, got %s", s.LocalRepository)
				}
			},
		},
		{
			name: "settings with proxies",
			input: `<settings>
  <proxies>
    <proxy>
      <id>test</id>
      <active>true</active>
      <protocol>http</protocol>
      <host>proxy.example.com</host>
      <port>3128</port>
    </proxy>
  </proxies>
</settings>`,
			wantErr: false,
			checks: func(t *testing.T, s *mavenSettings) {
				if s.Proxies == nil || len(s.Proxies.Proxy) != 1 {
					t.Fatal("expected 1 proxy entry")
				}
				if s.Proxies.Proxy[0].Host != "proxy.example.com" {
					t.Errorf("expected host proxy.example.com, got %s", s.Proxies.Proxy[0].Host)
				}
			},
		},
		{
			name: "settings with extra elements preserved",
			input: `<settings>
  <mirrors><mirror><id>central</id></mirror></mirrors>
  <localRepository>/tmp/repo</localRepository>
</settings>`,
			wantErr: false,
			checks: func(t *testing.T, s *mavenSettings) {
				if s.LocalRepository != "/tmp/repo" {
					t.Errorf("expected /tmp/repo, got %s", s.LocalRepository)
				}
				if len(s.Extra) != 1 {
					t.Fatalf("expected 1 extra element, got %d", len(s.Extra))
				}
				if s.Extra[0].XMLName.Local != "mirrors" {
					t.Errorf("expected extra element mirrors, got %s", s.Extra[0].XMLName.Local)
				}
			},
		},
		{
			name:    "malformed XML returns error",
			input:   `<settings><broken`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := parseMavenSettings([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checks != nil {
				tt.checks(t, s)
			}
		})
	}
}

func TestMarshalMavenSettings(t *testing.T) {
	s := &mavenSettings{
		LocalRepository: "/tmp/repo",
		Proxies: &mavenProxies{
			Proxy: []mavenProxyEntry{
				{
					ID:       "http-proxy-1",
					Active:   "true",
					Protocol: "http",
					Host:     "proxy.example.com",
					Port:     "3128",
				},
			},
		},
	}

	output, err := marshalMavenSettings(s)
	if err != nil {
		t.Fatalf("marshalMavenSettings() error: %v", err)
	}

	content := string(output)
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		"<localRepository>/tmp/repo</localRepository>",
		"<host>proxy.example.com</host>",
		"<port>3128</port>",
		`xmlns="http://maven.apache.org/SETTINGS/1.0.0"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("expected output to contain %q, got:\n%s", want, content)
		}
	}
}

func TestMarshalMavenSettingsPreservesExistingAttrs(t *testing.T) {
	s := &mavenSettings{
		Xmlns:          "http://custom.ns",
		Xsi:            "http://custom.xsi",
		SchemaLocation: "http://custom.schema",
	}

	output, err := marshalMavenSettings(s)
	if err != nil {
		t.Fatalf("marshalMavenSettings() error: %v", err)
	}

	content := string(output)
	if !strings.Contains(content, "http://custom.ns") {
		t.Errorf("expected custom xmlns preserved, got:\n%s", content)
	}
}

func TestBuildSettingsFile(t *testing.T) {
	p := &javaProvider{}

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	tests := []struct {
		name       string
		m2CacheDir string
		proxy      *provider.Proxy
		contains   []string
		excludes   []string
	}{
		{
			name:       "cache dir only, no proxy",
			m2CacheDir: "/custom/m2/repo",
			proxy:      nil,
			contains: []string{
				"<localRepository>/custom/m2/repo</localRepository>",
				"<settings",
				"</settings>",
			},
			excludes: []string{
				"<proxies>",
			},
		},
		{
			name:       "proxy only, no cache dir",
			m2CacheDir: "",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
			},
			contains: []string{
				"<proxies>",
				"<host>proxy.example.com</host>",
				"<port>3128</port>",
				"<settings",
			},
			excludes: []string{
				"<localRepository>",
			},
		},
		{
			name:       "both cache dir and proxy",
			m2CacheDir: "/custom/m2/repo",
			proxy: &provider.Proxy{
				HTTPProxy:  "http://proxy.example.com:3128",
				HTTPSProxy: "http://proxy.example.com:3129",
				NoProxy:    "localhost,127.0.0.1",
			},
			contains: []string{
				"<localRepository>/custom/m2/repo</localRepository>",
				"<proxies>",
				"<protocol>http</protocol>",
				"<protocol>https</protocol>",
				"<nonProxyHosts>localhost|127.0.0.1</nonProxyHosts>",
			},
		},
		{
			name:       "no cache dir, no proxy",
			m2CacheDir: "",
			proxy:      nil,
			contains: []string{
				"<settings",
				"</settings>",
			},
			excludes: []string{
				"<localRepository>",
				"<proxies>",
			},
		},
		{
			name:       "empty proxy struct treated as no proxy",
			m2CacheDir: "/custom/m2/repo",
			proxy:      &provider.Proxy{},
			contains: []string{
				"<localRepository>/custom/m2/repo</localRepository>",
				"<settings",
			},
			excludes: []string{
				"<proxies>",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Remove any existing file to isolate test cases
			os.Remove(filepath.Join(tmpDir, ".analyze", "globalSettings.xml"))

			path, err := p.BuildSettingsFile(tt.m2CacheDir, tt.proxy)
			if err != nil {
				t.Fatalf("BuildSettingsFile() error: %v", err)
			}
			if path == "" {
				t.Fatal("BuildSettingsFile() returned empty path")
			}

			expectedSuffix := filepath.Join(".analyze", "globalSettings.xml")
			if !strings.HasSuffix(path, expectedSuffix) {
				t.Errorf("expected path to end with %q, got %q", expectedSuffix, path)
			}

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read settings file: %v", err)
			}

			for _, s := range tt.contains {
				if !strings.Contains(string(content), s) {
					t.Errorf("expected file to contain %q, got:\n%s", s, string(content))
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(string(content), s) {
					t.Errorf("expected file to NOT contain %q, got:\n%s", s, string(content))
				}
			}
		})
	}
}

func TestBuildSettingsFileFallbackHome(t *testing.T) {
	p := &javaProvider{}

	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	path, err := p.BuildSettingsFile("/custom/repo", nil)
	if err != nil {
		t.Fatalf("BuildSettingsFile() error: %v", err)
	}

	expectedSuffix := filepath.Join(".analyze", "globalSettings.xml")
	if !strings.HasSuffix(path, expectedSuffix) {
		t.Errorf("expected path to end with %q, got %q", expectedSuffix, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}
	if !strings.Contains(string(content), "<localRepository>/custom/repo</localRepository>") {
		t.Errorf("expected localRepository in output, got:\n%s", string(content))
	}
}

func TestBuildSettingsFileMergesExistingFile(t *testing.T) {
	p := &javaProvider{}

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	analyzeDir := filepath.Join(tmpDir, ".analyze")
	os.MkdirAll(analyzeDir, 0777)
	settingsPath := filepath.Join(analyzeDir, "globalSettings.xml")

	tests := []struct {
		name       string
		existing   string
		m2CacheDir string
		proxy      *provider.Proxy
		contains   []string
		excludes   []string
	}{
		{
			name: "preserves mirrors when updating localRepository",
			existing: `<settings>
  <mirrors>
    <mirror>
      <id>central-mirror</id>
      <url>https://mirror.example.com/maven2</url>
      <mirrorOf>central</mirrorOf>
    </mirror>
  </mirrors>
  <localRepository>/old/repo</localRepository>
</settings>`,
			m2CacheDir: "/new/repo",
			proxy:      nil,
			contains: []string{
				"<localRepository>/new/repo</localRepository>",
				"central-mirror",
				"https://mirror.example.com/maven2",
			},
			excludes: []string{
				"/old/repo",
			},
		},
		{
			name: "preserves profiles when adding proxies",
			existing: `<settings>
  <profiles>
    <profile>
      <id>custom-profile</id>
    </profile>
  </profiles>
</settings>`,
			m2CacheDir: "",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
			},
			contains: []string{
				"custom-profile",
				"<proxies>",
				"<host>proxy.example.com</host>",
			},
		},
		{
			name: "replaces existing proxies with new ones",
			existing: `<settings>
  <proxies>
    <proxy>
      <id>old-proxy</id>
      <active>true</active>
      <protocol>http</protocol>
      <host>old-proxy.example.com</host>
      <port>8080</port>
    </proxy>
  </proxies>
</settings>`,
			m2CacheDir: "",
			proxy: &provider.Proxy{
				HTTPProxy: "http://new-proxy.example.com:3128",
			},
			contains: []string{
				"<host>new-proxy.example.com</host>",
				"<port>3128</port>",
			},
			excludes: []string{
				"old-proxy.example.com",
				"8080",
			},
		},
		{
			name: "preserves localRepository when only setting proxy",
			existing: `<settings>
  <localRepository>/existing/repo</localRepository>
</settings>`,
			m2CacheDir: "",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
			},
			contains: []string{
				"<localRepository>/existing/repo</localRepository>",
				"<proxies>",
			},
		},
		{
			name: "preserves existing proxies when only setting localRepository",
			existing: `<settings>
  <proxies>
    <proxy>
      <id>keep-me</id>
      <active>true</active>
      <protocol>http</protocol>
      <host>keep-proxy.example.com</host>
      <port>3128</port>
    </proxy>
  </proxies>
</settings>`,
			m2CacheDir: "/new/repo",
			proxy:      nil,
			contains: []string{
				"<localRepository>/new/repo</localRepository>",
				"keep-proxy.example.com",
			},
		},
		{
			name: "updates both localRepository and proxies",
			existing: `<settings>
  <localRepository>/old/repo</localRepository>
  <mirrors>
    <mirror><id>m1</id></mirror>
  </mirrors>
  <proxies>
    <proxy>
      <id>old</id>
      <host>old.example.com</host>
    </proxy>
  </proxies>
</settings>`,
			m2CacheDir: "/new/repo",
			proxy: &provider.Proxy{
				HTTPProxy: "http://new.example.com:8080",
			},
			contains: []string{
				"<localRepository>/new/repo</localRepository>",
				"<host>new.example.com</host>",
				"m1",
			},
			excludes: []string{
				"/old/repo",
				"old.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(settingsPath, []byte(tt.existing), 0644)
			if err != nil {
				t.Fatalf("failed to write existing settings: %v", err)
			}

			path, err := p.BuildSettingsFile(tt.m2CacheDir, tt.proxy)
			if err != nil {
				t.Fatalf("BuildSettingsFile() error: %v", err)
			}

			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read settings file: %v", err)
			}

			for _, s := range tt.contains {
				if !strings.Contains(string(content), s) {
					t.Errorf("expected file to contain %q, got:\n%s", s, string(content))
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(string(content), s) {
					t.Errorf("expected file to NOT contain %q, got:\n%s", s, string(content))
				}
			}
		})
	}
}

func TestBuildSettingsFileMalformedExistingFile(t *testing.T) {
	p := &javaProvider{}

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	analyzeDir := filepath.Join(tmpDir, ".analyze")
	os.MkdirAll(analyzeDir, 0777)
	settingsPath := filepath.Join(analyzeDir, "globalSettings.xml")

	// Write malformed XML
	err := os.WriteFile(settingsPath, []byte("<settings><broken"), 0644)
	if err != nil {
		t.Fatalf("failed to write malformed settings: %v", err)
	}

	// Should start fresh instead of failing
	path, err := p.BuildSettingsFile("/custom/repo", nil)
	if err != nil {
		t.Fatalf("BuildSettingsFile() should handle malformed XML gracefully, got error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	if !strings.Contains(string(content), "<localRepository>/custom/repo</localRepository>") {
		t.Errorf("expected localRepository in output, got:\n%s", string(content))
	}
}

func TestBuildSettingsFileOutputIsValidXML(t *testing.T) {
	p := &javaProvider{}

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	path, err := p.BuildSettingsFile("/custom/repo", &provider.Proxy{
		HTTPProxy:  "http://user:p%40ss@proxy.example.com:3128",
		HTTPSProxy: "https://proxy.example.com:3129",
		NoProxy:    "localhost,127.0.0.1",
	})
	if err != nil {
		t.Fatalf("BuildSettingsFile() error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	// Verify the output is valid XML by parsing it
	var s mavenSettings
	if err := xml.Unmarshal(content, &s); err != nil {
		t.Fatalf("output is not valid XML: %v\nContent:\n%s", err, string(content))
	}

	if s.LocalRepository != "/custom/repo" {
		t.Errorf("expected localRepository /custom/repo, got %s", s.LocalRepository)
	}
	if s.Proxies == nil || len(s.Proxies.Proxy) != 2 {
		t.Fatalf("expected 2 proxy entries, got %v", s.Proxies)
	}
}

func TestBuildSettingsFileRoundTrip(t *testing.T) {
	p := &javaProvider{}

	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)
	t.Setenv("HOME", tmpDir)

	// First call creates file
	_, err := p.BuildSettingsFile("/first/repo", &provider.Proxy{
		HTTPProxy: "http://first-proxy.example.com:3128",
	})
	if err != nil {
		t.Fatalf("first BuildSettingsFile() error: %v", err)
	}

	// Second call should merge into existing file
	path, err := p.BuildSettingsFile("/second/repo", &provider.Proxy{
		HTTPSProxy: "http://second-proxy.example.com:3129",
	})
	if err != nil {
		t.Fatalf("second BuildSettingsFile() error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}

	contentStr := string(content)

	// localRepository should be updated to second value
	if !strings.Contains(contentStr, "/second/repo") {
		t.Errorf("expected /second/repo, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "/first/repo") {
		t.Errorf("expected /first/repo to be replaced, got:\n%s", contentStr)
	}

	// Proxy should be updated to second value
	if !strings.Contains(contentStr, "second-proxy.example.com") {
		t.Errorf("expected second-proxy.example.com, got:\n%s", contentStr)
	}
	if strings.Contains(contentStr, "first-proxy.example.com") {
		t.Errorf("expected first-proxy.example.com to be replaced, got:\n%s", contentStr)
	}
}
