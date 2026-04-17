//go:build integration

// Run with: go test -tags=integration ./pkg/nodejs_external_provider/...
// Requires typescript-language-server on PATH and examples/nodejs.

package nodejs_external_provider_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	nodejsext "github.com/konveyor/analyzer-lsp/external-providers/nodejs-external-provider/pkg/nodejs_external_provider"
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

func TestTypescriptLSInitAndEvaluate(t *testing.T) {
	if _, err := exec.LookPath("typescript-language-server"); err != nil {
		t.Skip("skipping: typescript-language-server not on PATH")
	}

	logrusLog := logrus.New()
	logrusLog.SetOutput(testingWriter{t})
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))
	log := logrusr.New(logrusLog)

	prov := nodejsext.NewNodejsProvider("nodejs", log, nil)

	repoRoot := testRepoRoot(t)
	nodeDir := filepath.Join(repoRoot, "examples", "nodejs")
	nodeFolderURI := string(uri.File(nodeDir))

	nodeSC, _, err := prov.Init(context.Background(), log, provider.InitConfig{
		Location: nodeDir,
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":     "nodejs",
			"lspServerPath":     "typescript-language-server",
			"lspServerArgs":     []interface{}{"--stdio"},
			"workspaceFolders":  []interface{}{nodeFolderURI},
			"dependencyFolders": []interface{}{},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	filePath := filepath.Join(nodeDir, "test_a.ts")
	docURI := string(uri.File(filePath))
	src, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	didOpen := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        docURI,
			LanguageID: "typescript",
			Version:    0,
			Text:       string(src),
		},
	}
	err = nodeSC.(*nodejsext.NodeServiceClient).Conn.Notify(context.Background(), "textDocument/didOpen", didOpen)
	if err != nil {
		t.Fatal(err)
	}

	calls := []struct {
		cap  string
		info []byte
	}{
		{"referenced", []byte(`{"referenced":{"pattern": "hello"}}`)},
		{"referenced", []byte(`{"referenced":{"pattern": "greeter"}}`)},
	}
	for _, call := range calls {
		response, err := nodeSC.Evaluate(context.Background(), call.cap, call.info)
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
