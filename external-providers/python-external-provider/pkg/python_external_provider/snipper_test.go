package python_external_provider_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-logr/logr"
	pythonext "github.com/konveyor/analyzer-lsp/external-providers/python-external-provider/pkg/python_external_provider"
	"github.com/konveyor/analyzer-lsp/engine"
	"go.lsp.dev/uri"
)

func TestGetCodeSnip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.py")
	content := "import os\n\nprint('hello')\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	p := pythonext.NewPythonProvider("python", logr.Discard(), 10, nil)
	snipper, ok := p.(engine.CodeSnip)
	if !ok {
		t.Fatal("python provider does not implement engine.CodeSnip")
	}

	snip, err := snipper.GetCodeSnip(uri.File(path), engine.Location{
		StartPosition: engine.Position{Line: 2},
		EndPosition:   engine.Position{Line: 2},
	})
	if err != nil {
		t.Fatalf("GetCodeSnip: %v", err)
	}
	want := " 1  import os\n 2  \n 3  print('hello')\n"
	if snip != want {
		t.Errorf("GetCodeSnip = %q, want %q", snip, want)
	}
}

func TestGetCodeSnipInvalidURI(t *testing.T) {
	p := pythonext.NewPythonProvider("python", logr.Discard(), 10, nil)
	snipper, ok := p.(engine.CodeSnip)
	if !ok {
		t.Fatal("python provider does not implement engine.CodeSnip")
	}

	_, err := snipper.GetCodeSnip("jdt://some/path", engine.Location{})
	if err == nil {
		t.Fatal("expected error for non-file URI")
	}
}
