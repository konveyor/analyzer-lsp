package provider

import (
	"context"
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
