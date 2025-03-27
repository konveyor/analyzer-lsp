package nodejs_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/generic_external_provider"
	"github.com/konveyor/analyzer-lsp/external-providers/generic-external-provider/pkg/server_configurations/nodejs"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"gopkg.in/yaml.v2"

	"github.com/sirupsen/logrus"
)

// For debugging
type EvaluateCall struct {
	ServiceClient provider.ServiceClient
	Cap           string
	ConditionInfo []byte
}

func TestNodeServiceClient(t *testing.T) {
	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))
	log := logrusr.New(logrusLog)
	ctx := context.TODO()

	prov := generic_external_provider.NewGenericProvider("nodejs", log)

	nodeSC, _, err := prov.Init(ctx, log, provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":     "nodejs",
			"lspServerPath":     "/usr/local/bin/typescript-language-server",
			"lspServerArgs":     []interface{}{"--stdio"},
			"workspaceFolders":  []interface{}{"file:///Users/emilymcmullan/Repos/analyzer-lsp/examples/nodejs"},
			"dependencyFolders": []interface{}{},
		},
	})
	_ = nodeSC

	if err != nil {
		fmt.Printf("%v\n", err)
		panic(err)
	}

	var res json.RawMessage
	_ = res

	f, err := os.ReadFile("/Users/emilymcmullan/Repos/analyzer-lsp/examples/nodejs/test.ts")
	if err != nil {
		panic(err)
	}

	didOpen := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "file:///Users/emilymcmullan/Repos/analyzer-lsp/examples/nodejs/test.ts",
			LanguageID: "typescript",
			Version:    0,
			Text:       string(f),
		},
	}
	err = nodeSC.(*nodejs.NodeServiceClient).Conn.Notify(ctx, "textDocument/didOpen", didOpen)
	if err != nil {
		panic(err)
	}

	var calls []EvaluateCall
	calls = append(
		calls,
		EvaluateCall{nodeSC, "referenced", []byte(`{"referenced":{pattern: "testFunc"}}`)},
		EvaluateCall{nodeSC, "referenced", []byte(`{"referenced":{pattern: "test"}}`)},
	)

	for _, call := range calls {
		response, err := call.ServiceClient.Evaluate(ctx, call.Cap, call.ConditionInfo)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Service Client: %s\n", call.ServiceClient.(*nodejs.NodeServiceClient).BaseConfig.LspServerName)
		fmt.Printf("Evaluated: %s, %s\n", call.Cap, string(call.ConditionInfo))
		fmt.Printf("Incidents:\n")
		b, _ := yaml.Marshal(response.Incidents)
		s := string(b)
		fmt.Printf("%s\n", s)

		fmt.Printf("Matched: %v\n", response.Matched)
		fmt.Printf("TemplateContext: %v\n", response.TemplateContext)
	}
}
