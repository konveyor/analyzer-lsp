package java

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/konveyor/analyzer-lsp/provider"
)

// mavenSettings represents the top-level Maven settings.xml structure.
// The Extra field captures any XML elements we don't explicitly model,
// so they are preserved when reading and rewriting an existing file.
type mavenSettings struct {
	XMLName           xml.Name          `xml:"settings"`
	Xmlns             string            `xml:"xmlns,attr,omitempty"`
	XmlnsXsi          string            `xml:"xmlns:xsi,attr,omitempty"`
	XsiSchemaLocation string            `xml:"xsi:schemaLocation,attr,omitempty"`
	LocalRepository   string            `xml:"localRepository,omitempty"`
	Proxies           *mavenProxies     `xml:"proxies,omitempty"`
	Extra             []mavenExtraEntry `xml:",any"`
}

type mavenProxies struct {
	Proxy []mavenProxyEntry `xml:"proxy"`
}

type mavenProxyEntry struct {
	ID            string `xml:"id,omitempty"`
	Active        string `xml:"active,omitempty"`
	Protocol      string `xml:"protocol,omitempty"`
	Host          string `xml:"host,omitempty"`
	Port          string `xml:"port,omitempty"`
	Username      string `xml:"username,omitempty"`
	Password      string `xml:"password,omitempty"`
	NonProxyHosts string `xml:"nonProxyHosts,omitempty"`
}

// mavenExtraEntry captures unknown XML elements so they survive round-tripping.
type mavenExtraEntry struct {
	XMLName xml.Name
	Content string `xml:",innerxml"`
}

// parseMavenSettings parses a Maven settings.xml from the given bytes.
func parseMavenSettings(data []byte) (*mavenSettings, error) {
	var s mavenSettings
	if err := xml.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// buildMavenProxyEntries builds structured proxy entries from provider.Proxy config.
func buildMavenProxyEntries(proxy *provider.Proxy) []mavenProxyEntry {
	if proxy == nil {
		return nil
	}
	var entries []mavenProxyEntry
	id := 1
	if proxy.HTTPProxy != "" {
		if e, ok := buildMavenProxyEntryStruct(proxy.HTTPProxy, "http", id, proxy.NoProxy); ok {
			entries = append(entries, e)
			id++
		}
	}
	if proxy.HTTPSProxy != "" {
		if e, ok := buildMavenProxyEntryStruct(proxy.HTTPSProxy, "https", id, proxy.NoProxy); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

// buildMavenProxyEntryStruct creates a single mavenProxyEntry from a proxy URL.
func buildMavenProxyEntryStruct(proxyURL, protocol string, id int, noProxy string) (mavenProxyEntry, bool) {
	u, err := url.Parse(proxyURL)
	if err != nil || u.Host == "" {
		return mavenProxyEntry{}, false
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	entry := mavenProxyEntry{
		ID:       fmt.Sprintf("%s-proxy-%d", protocol, id),
		Active:   "true",
		Protocol: protocol,
		Host:     host,
		Port:     port,
	}
	if u.User != nil {
		entry.Username = u.User.Username()
		if pw, ok := u.User.Password(); ok {
			entry.Password = pw
		}
	}
	if noProxy != "" {
		parts := strings.Split(noProxy, ",")
		normalized := make([]string, 0, len(parts))
		for _, p := range parts {
			if v := strings.TrimSpace(p); v != "" {
				normalized = append(normalized, v)
			}
		}
		entry.NonProxyHosts = strings.Join(normalized, "|")
	}
	return entry, true
}

// marshalMavenSettings serializes a mavenSettings struct to indented XML with
// the standard XML declaration header.
func marshalMavenSettings(s *mavenSettings) ([]byte, error) {
	if s.Xmlns == "" {
		s.Xmlns = "http://maven.apache.org/SETTINGS/1.0.0"
	}
	// encoding/xml doesn't populate xmlns:xsi and xsi:schemaLocation on
	// unmarshal, so always set them to ensure valid Maven output.
	s.XmlnsXsi = "http://www.w3.org/2001/XMLSchema-instance"
	s.XsiSchemaLocation = "http://maven.apache.org/SETTINGS/1.0.0 https://maven.apache.org/xsd/settings-1.0.0.xsd"
	output, err := xml.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append([]byte(xml.Header), append(output, '\n')...), nil
}

// BuildSettingsFile creates or updates the Maven globalSettings.xml file.
// If the file already exists, it is parsed and only the localRepository and
// proxies sections are updated; all other settings are preserved.
// If the file doesn't exist or is malformed, a new one is created.
func (p *javaProvider) BuildSettingsFile(m2CacheDir string, proxy *provider.Proxy) (settingsFile string, err error) {
	var homeDir string
	set := true
	ops := runtime.GOOS
	if ops == "linux" {
		homeDir, set = os.LookupEnv("XDG_CONFIG_HOME")
	}
	if ops != "linux" || homeDir == "" || !set {
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return "", err
		}
	}
	settingsFilePath := filepath.Join(homeDir, ".analyze", "globalSettings.xml")
	err = os.MkdirAll(filepath.Dir(settingsFilePath), 0755)
	if err != nil {
		return "", err
	}

	// Try to read and parse existing settings file
	var settings *mavenSettings
	if data, readErr := os.ReadFile(settingsFilePath); readErr == nil {
		settings, err = parseMavenSettings(data)
		if err != nil {
			// Existing file is malformed; start fresh
			settings = nil
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return "", readErr
	}
	if settings == nil {
		settings = &mavenSettings{}
	}

	// Update localRepository if requested
	if m2CacheDir != "" {
		settings.LocalRepository = m2CacheDir
	}

	// Update proxies if requested
	if proxy != nil {
		entries := buildMavenProxyEntries(proxy)
		if len(entries) > 0 {
			settings.Proxies = &mavenProxies{Proxy: entries}
		} else {
			settings.Proxies = nil
		}
	}

	output, err := marshalMavenSettings(settings)
	if err != nil {
		return "", err
	}

	err = os.WriteFile(settingsFilePath, output, 0600)
	if err != nil {
		return "", err
	}

	return settingsFilePath, nil
}

// redactProxyURL removes credentials from a proxy URL for safe logging.
func redactProxyURL(proxyURL string) string {
	if proxyURL == "" {
		return ""
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return "[invalid URL]"
	}
	if u.User != nil {
		u.User = url.User("[REDACTED]")
	}
	return u.String()
}
