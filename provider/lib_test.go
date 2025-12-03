package provider

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMultilineGrep(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		pattern  string
		window   int
		want     int
		wantErr  bool
	}{
		{
			name:     "plain single line text",
			filePath: "./testdata/small.xml",
			pattern:  "xmlns:xsi=\"http://www.w3.org/2001/XMLSchema-instance\"",
			want:     2,
			window:   1,
			wantErr:  false,
		},
		{
			name:     "multi-line simple pattern",
			filePath: "./testdata/small.xml",
			pattern:  "com.fasterxml.jackson.core.*?jackson-core.*",
			want:     68,
			window:   2,
			wantErr:  false,
		},
		{
			name:     "multi-line complex pattern",
			filePath: "./testdata/small.xml",
			pattern:  "(<groupId>com.fasterxml.jackson.core</groupId>|<artifactId>jackson-core</artifactId>).*?(<artifactId>jackson-core</artifactId>|<groupId>com.fasterxml.jackson.core</groupId>).*",
			want:     68,
			window:   2,
			wantErr:  false,
		},
		{
			name:     "multi-line complex pattern",
			filePath: "./testdata/big.xml",
			pattern:  "(<groupId>io.konveyor.demo</groupId>|<artifactId>config-utils</artifactId>).*?(<artifactId>config-utils</artifactId>|<groupId>io.konveyor.demo</groupId>).*",
			want:     664,
			window:   2,
			wantErr:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MultilineGrep(context.TODO(), tt.window, tt.filePath, tt.pattern)
			if (err != nil) != tt.wantErr {
				t.Errorf("MultilineGrep() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("MultilineGrep() = %v, want %v", got, tt.want)
			}
		})
	}
}

func BenchmarkMultilineGrepFileSizeSmall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, canMe := context.WithTimeout(context.TODO(), time.Second*3)
		MultilineGrep(ctx, 5,
			"./testdata/small.xml",
			"(<groupId>com.fasterxml.jackson.core</groupId>|<artifactId>jackson-core</artifactId>).*?(<artifactId>jackson-core</artifactId>|<groupId>com.fasterxml.jackson.core</groupId>).*")
		canMe()
	}
}

func BenchmarkMultilineGrepFileSizeBig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ctx, canMe := context.WithTimeout(context.TODO(), time.Second*3)
		MultilineGrep(ctx, 5,
			"./testdata/big.xml",
			"(<groupId>io.konveyor.demo</groupId>|<artifactId>config-utils</artifactId>).*?(<artifactId>config-utils</artifactId>|<groupId>io.konveyor.demo</groupId>).*")
		canMe()
	}
}

func TestNormalizePathForComparison(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "file:// URI scheme",
			input:    "file:///project/src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "file: URI scheme",
			input:    "file:/project/src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "plain path",
			input:    "/project/src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "path with ..",
			input:    "/project/src/../src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "path with .",
			input:    "/project/./src/Main.java",
			expected: "/project/src/Main.java",
		},
		{
			name:     "windows-style path",
			input:    "file:///C:/project/src/Main.java",
			expected: "/C:/project/src/Main.java",
		},
		{
			name:     "csharp metadata URI",
			input:    "csharp:/metadata/projects/MyApp/assemblies/System.Web.Mvc/symbols/Controller.cs",
			expected: "csharp:/metadata/projects/MyApp/assemblies/System.Web.Mvc/symbols/Controller.cs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePathForComparison(tt.input)
			expected := tt.expected
			// On Windows, paths are normalized to lowercase (except csharp: URIs)
			if runtime.GOOS == "windows" && !strings.HasPrefix(tt.input, "csharp:") {
				expected = strings.ToLower(expected)
			}
			if result != expected {
				t.Errorf("NormalizePathForComparison(%q) = %q, want %q", tt.input, result, expected)
			}
		})
	}
}
