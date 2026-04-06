//go:build integration

// Run with: go test -tags=integration ./pkg/go_external_provider/...
// Requires gopls on PATH and network/filesystem access to examples.

package go_external_provider_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	goext "github.com/konveyor/analyzer-lsp/external-providers/go-external-provider/pkg/go_external_provider"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

func testRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// .../pkg/go_external_provider -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../.."))
}

func TestGoplsInitAndEvaluate(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetOutput(testingWriter{t})
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))
	log := logrusr.New(logrusLog)

	prov := goext.NewGoProvider("go", log, nil)

	repoRoot := testRepoRoot(t)
	goplsExamples := "file://" + filepath.Join(repoRoot, "examples", "golang")

	goplsSC, _, err := prov.Init(context.Background(), log, provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":    "go",
			"lspServerPath":    "gopls",
			"lspServerArgs":    []interface{}{"-vv", "-logfile", "debug-go.log", "-rpc.trace"},
			"workspaceFolders": []interface{}{goplsExamples},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var res json.RawMessage
	p0 := protocol.WorkspaceSymbolParams{
		Query: "dummy.HelloWorld",
	}
	err = goplsSC.(*goext.GoServiceClient).Conn.Call(context.Background(), "workspace/symbol", p0).Await(context.Background(), &res)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("workspace/symbol: %s", string(res))

	calls := []struct {
		cap  string
		info []byte
	}{
		{"referenced", []byte(`{"referenced":{"pattern": "HelloWorld"}}`)},
		{"echo", []byte(`{"echo":{"input": "what's up!"}}`)},
	}
	for _, call := range calls {
		response, err := goplsSC.Evaluate(context.Background(), call.cap, call.info)
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
