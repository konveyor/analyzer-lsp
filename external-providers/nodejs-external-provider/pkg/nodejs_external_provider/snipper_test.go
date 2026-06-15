package nodejs_external_provider_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	nodejsext "github.com/konveyor/analyzer-lsp/external-providers/nodejs-external-provider/pkg/nodejs_external_provider"
	"github.com/konveyor/analyzer-lsp/engine"
	"go.lsp.dev/uri"
)

func TestGetCodeSnip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.js")
	content := "const fs = require('fs');\n\nconsole.log('hello');\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := nodejsext.NewNodejsProvider("nodejs", logr.Discard(), 10, nil)
	snipper, ok := p.(engine.CodeSnip)
	if !ok {
		t.Fatal("nodejs provider does not implement engine.CodeSnip")
	}

	snip, err := snipper.GetCodeSnip(uri.File(path), engine.Location{
		StartPosition: engine.Position{Line: 2},
		EndPosition:   engine.Position{Line: 2},
	})
	if err != nil {
		t.Fatalf("GetCodeSnip: %v", err)
	}
	want := " 1  const fs = require('fs');\n 2  \n 3  console.log('hello');\n"
	if snip != want {
		t.Errorf("GetCodeSnip = %q, want %q", snip, want)
	}
}

func TestGetCodeSnipInvalidURI(t *testing.T) {
	p := nodejsext.NewNodejsProvider("nodejs", logr.Discard(), 10, nil)
	snipper, ok := p.(engine.CodeSnip)
	if !ok {
		t.Fatal("nodejs provider does not implement engine.CodeSnip")
	}

	_, err := snipper.GetCodeSnip("jdt://some/path", engine.Location{})
	if err == nil {
		t.Fatal("expected error for non-file URI")
	}
}
