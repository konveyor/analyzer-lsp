//go:build integration

// Run with: go test -tags=integration ./pkg/python_external_provider/...
// Requires pylsp on PATH and filesystem access to examples/python.

package python_external_provider_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	pythonext "github.com/konveyor/analyzer-lsp/external-providers/python-external-provider/pkg/python_external_provider"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
	"go.lsp.dev/uri"
	"gopkg.in/yaml.v2"
)

func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func TestPylspInitAndEvaluate(t *testing.T) {
	if _, err := exec.LookPath("pylsp"); err != nil {
		t.Skip("skipping: pylsp not on PATH")
	}

	logrusLog := logrus.New()
	logrusLog.SetOutput(testingWriter{t})
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))
	log := logrusr.New(logrusLog)

	prov := pythonext.NewPythonProvider("pylsp", log, nil)

	repoRoot := testRepoRoot(t)
	pyDir := filepath.Join(repoRoot, "examples", "python")
	pyFolderURI := string(uri.File(pyDir))

	pylspSC, _, err := prov.Init(context.Background(), log, provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":    "pylsp",
			"lspServerPath":    "pylsp",
			"lspServerArgs":    []interface{}{},
			"workspaceFolders": []interface{}{pyFolderURI},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(pyDir, "file_b.py")
	docURI := string(uri.File(filePath))
	src, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	didOpen := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        docURI,
			LanguageID: "python",
			Version:    0,
			Text:       string(src),
		},
	}
	err = pylspSC.(*pythonext.PythonServiceClient).Conn.Notify(context.Background(), "textDocument/didOpen", didOpen)
	if err != nil {
		t.Fatal(err)
	}

	_ = pylspSC.(*pythonext.PythonServiceClient).
		PublishDiagnosticsCache.
		Get(docURI).
		Await()

	calls := []struct {
		cap  string
		info []byte
	}{
		{"referenced", []byte(`{"referenced":{"pattern": "hello_world"}}`)},
		{"referenced", []byte(`{"referenced":{"pattern": "speak"}}`)},
	}
	for _, call := range calls {
		response, err := pylspSC.Evaluate(context.Background(), call.cap, call.info)
		if err != nil {
			t.Fatalf("Evaluate %s: %v", call.cap, err)
		}
		t.Logf("cap=%s matched=%v incidents=%v", call.cap, response.Matched, mustYAML(t, response.Incidents))
	}
}

type testingWriter struct{ t *testing.T }

func (w testingWriter) Write(p []byte) (n int, err error) {
	w.t.Helper()
	w.t.Logf("%s", p)
	return len(p), nil
}

func mustYAML(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := yaml.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
