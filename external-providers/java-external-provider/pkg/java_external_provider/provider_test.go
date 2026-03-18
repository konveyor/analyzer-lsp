package java

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/konveyor/analyzer-lsp/provider"
)

func TestBuildMavenProxies(t *testing.T) {
	tests := []struct {
		name     string
		proxy    *provider.Proxy
		wantNil  bool
		contains []string
		excludes []string
	}{
		{
			name:    "nil proxy returns empty",
			proxy:   nil,
			wantNil: true,
		},
		{
			name: "http proxy only",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
			},
			contains: []string{
				"<proxies>",
				"<protocol>http</protocol>",
				"<host>proxy.example.com</host>",
				"<port>3128</port>",
				"<active>true</active>",
			},
			excludes: []string{
				"<protocol>https</protocol>",
			},
		},
		{
			name: "https proxy only",
			proxy: &provider.Proxy{
				HTTPSProxy: "http://proxy.example.com:3129",
			},
			contains: []string{
				"<proxies>",
				"<protocol>https</protocol>",
				"<host>proxy.example.com</host>",
				"<port>3129</port>",
			},
			excludes: []string{
				"<protocol>http</protocol>",
			},
		},
		{
			name: "both http and https proxies",
			proxy: &provider.Proxy{
				HTTPProxy:  "http://proxy.example.com:3128",
				HTTPSProxy: "http://proxy.example.com:3129",
			},
			contains: []string{
				"<protocol>http</protocol>",
				"<protocol>https</protocol>",
				"http-proxy-1",
				"https-proxy-2",
			},
		},
		{
			name: "proxy with authentication",
			proxy: &provider.Proxy{
				HTTPProxy: "http://user:pass@proxy.example.com:3128",
			},
			contains: []string{
				"<username>user</username>",
				"<password>pass</password>",
				"<host>proxy.example.com</host>",
			},
		},
		{
			name: "proxy with noProxy converts comma to pipe",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com:3128",
				NoProxy:   "localhost,127.0.0.1,.example.com",
			},
			contains: []string{
				"<nonProxyHosts>localhost|127.0.0.1|.example.com</nonProxyHosts>",
			},
		},
		{
			name: "default port for http",
			proxy: &provider.Proxy{
				HTTPProxy: "http://proxy.example.com",
			},
			contains: []string{
				"<port>80</port>",
			},
		},
		{
			name: "default port for https scheme",
			proxy: &provider.Proxy{
				HTTPSProxy: "https://proxy.example.com",
			},
			contains: []string{
				"<port>443</port>",
			},
		},
		{
			name: "empty proxy struct returns empty",
			proxy: &provider.Proxy{},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMavenProxies(tt.proxy)
			if tt.wantNil {
				if result != "" {
					t.Errorf("expected empty string, got %q", result)
				}
				return
			}
			for _, s := range tt.contains {
				if !strings.Contains(result, s) {
					t.Errorf("expected result to contain %q, got:\n%s", s, result)
				}
			}
			for _, s := range tt.excludes {
				if strings.Contains(result, s) {
					t.Errorf("expected result to NOT contain %q, got:\n%s", s, result)
				}
			}
		})
	}
}

func TestBuildProxyEntry(t *testing.T) {
	tests := []struct {
		name     string
		proxyURL string
		protocol string
		id       int
		noProxy  string
		want     string
		wantEmpty bool
	}{
		{
			name:      "invalid URL returns empty",
			proxyURL:  "://bad",
			protocol:  "http",
			id:        1,
			wantEmpty: true,
		},
		{
			name:      "empty URL returns empty",
			proxyURL:  "",
			protocol:  "http",
			id:        1,
			wantEmpty: true,
		},
		{
			name:     "bare hostname without scheme treated as path not host",
			proxyURL: "not-a-url",
			protocol: "http",
			id:       1,
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildProxyEntry(tt.proxyURL, tt.protocol, tt.id, tt.noProxy)
			if tt.wantEmpty {
				if result != "" {
					t.Errorf("expected empty string, got %q", result)
				}
				return
			}
			if !strings.Contains(result, tt.want) {
				t.Errorf("expected result to contain %q, got:\n%s", tt.want, result)
			}
		})
	}
}

func TestBuildSettingsFile(t *testing.T) {
	p := &javaProvider{}

	// Use a temp dir to avoid polluting real home
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	tests := []struct {
		name        string
		m2CacheDir  string
		proxy       *provider.Proxy
		contains    []string
		excludes    []string
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
			path, err := p.BuildSettingsFile(tt.m2CacheDir, tt.proxy)
			if err != nil {
				t.Fatalf("BuildSettingsFile() error: %v", err)
			}
			if path == "" {
				t.Fatal("BuildSettingsFile() returned empty path")
			}

			expectedPath := filepath.Join(tmpDir, ".analyze", "globalSettings.xml")
			if path != expectedPath {
				t.Errorf("expected path %q, got %q", expectedPath, path)
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

	// Unset XDG_CONFIG_HOME to trigger UserHomeDir fallback
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	t.Setenv("XDG_CONFIG_HOME", "")

	path, err := p.BuildSettingsFile("/custom/repo", nil)
	if err != nil {
		t.Fatalf("BuildSettingsFile() error: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".analyze", "globalSettings.xml")
	if path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, path)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings file: %v", err)
	}
	if !strings.Contains(string(content), "<localRepository>/custom/repo</localRepository>") {
		t.Errorf("expected localRepository in output, got:\n%s", string(content))
	}
}
