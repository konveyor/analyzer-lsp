package pylsp_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/getkin/kin-openapi/openapi3gen"
	"github.com/konveyor/analyzer-lsp/lsp/protocol"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/generic_external_provider"
	"github.com/konveyor/generic-external-provider/pkg/server_configurations/generic"
	"gopkg.in/yaml.v2"

	"github.com/sirupsen/logrus"
)

// For debugging
type EvaluateCall struct {
	ServiceClient provider.ServiceClient
	Cap           string
	ConditionInfo []byte
}

func TestHopefullyNothingBroke(t *testing.T) {
	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))
	log := logrusr.New(logrusLog)
	ctx := context.TODO()

	prov := generic_external_provider.NewGenericProvider("generic")

	pylspSC, err := prov.Init(ctx, log, provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":     "generic",
			"lspServerPath":     "pylsp",
			"lspServerArgs":     []interface{}{"--log-file", "python-debug.log"},
			"workspaceFolders":  []interface{}{"file:///home/jonah/Projects/analyzer-lsp/examples/python"},
			"dependencyFolders": []interface{}{"file:///home/jonah/Projects/analyzer-lsp/examples/python/.venv"},
		},
	})
	_ = pylspSC

	if err != nil {
		fmt.Printf("%v\n", err)
		panic(err)
	}

	var res json.RawMessage
	_ = res

	f, err := os.ReadFile("/home/jonah/Projects/analyzer-lsp/examples/python/file_b.py")
	if err != nil {
		panic(err)
	}

	didOpen := protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        "/home/jonah/Projects/analyzer-lsp/examples/python/file_b.py",
			LanguageID: "python",
			Version:    0,
			Text:       string(f),
		},
	}
	err = pylspSC.(*generic.GenericServiceClient).Conn.Notify(ctx, "textDocument/didOpen", didOpen)
	if err != nil {
		panic(err)
	}

	diagnostics := pylspSC.(*generic.GenericServiceClient).
		PublishDiagnosticsCache.
		Get("/home/jonah/Projects/analyzer-lsp/examples/python/file_b.py").
		Await()

	fmt.Printf("Diagnostics: %v\n", diagnostics)

	var calls []EvaluateCall
	calls = append(
		calls,
		EvaluateCall{pylspSC, "referenced", []byte(`{"referenced":{pattern: "def hello_world"}}`)},
		EvaluateCall{pylspSC, "referenced", []byte(`{"referenced":{pattern: "speak"}}`)},
		EvaluateCall{pylspSC, "referenced", []byte(`{"referenced":{pattern: "create_custom_resource_definition"}}`)},
	)

	for _, call := range calls {
		response, err := call.ServiceClient.Evaluate(ctx, call.Cap, call.ConditionInfo)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Service Client: %s\n", call.ServiceClient.(*generic.GenericServiceClient).BaseConfig.LspServerName)
		fmt.Printf("Evaluated: %s, %s\n", call.Cap, string(call.ConditionInfo))
		fmt.Printf("Incidents:\n")
		b, _ := yaml.Marshal(response.Incidents)
		s := string(b)
		fmt.Printf("%s\n", s)

		fmt.Printf("Matched: %v\n", response.Matched)
		fmt.Printf("TemplateContext: %v\n", response.TemplateContext)
	}
}

type someStruct struct {
	Bool    bool                      `json:"bool"`
	Int     int                       `json:"int"`
	Int64   int64                     `json:"int64"`
	Float64 float64                   `json:"float64"`
	String  string                    `json:"string"`
	Bytes   []byte                    `json:"bytes"`
	JSON    json.RawMessage           `json:"json"`
	Time    time.Time                 `json:"time"`
	Slice   []someOtherType           `json:"slice"`
	Map     map[string]*someOtherType `json:"map"`

	Struct struct {
		X string `json:"x"`
	} `json:"struct"`

	EmptyStruct struct {
		Y string
	} `json:"structWithoutFields"`

	Ptr *someOtherType `json:"ptr"`
}

// Example condition
type echoCondition struct {
	Echo struct {
		Input string `json:"input"`
	} `json:"echo"`
}

type someOtherType string

func XTestSchemaGen(t *testing.T) {
	r0, _ := openapi3gen.NewSchemaRefForValue(someStruct{}, nil)
	b0, _ := json.Marshal(*r0)

	r1, _ := openapi3gen.NewSchemaRefForValue(&someStruct{}, nil)
	b1, _ := json.Marshal(r1)

	fmt.Printf("%s\n", string(b0))
	fmt.Printf("%s\n", string(b1))

	if string(b0) != string(b1) {
		panic(1)
	}

	e2_before := echoCondition{
		Echo: struct {
			Input string `json:"input"`
		}{
			Input: "hello!",
		},
	}
	r2, _ := openapi3gen.NewSchemaRefForValue(e2_before, nil)
	b2, _ := json.Marshal(*r2)
	fmt.Printf("%s\n", string(b2))

	b2_yaml, _ := yaml.Marshal(e2_before)
	fmt.Printf("%s\n", string(b2_yaml))
	e2_after := echoCondition{}
	yaml.Unmarshal(b2_yaml, &e2_after)
	fmt.Printf("%v\n", e2_after)

	r3, _ := openapi3gen.NewSchemaRefForValue(struct{}{}, nil)
	b3, _ := json.Marshal(*r3)
	fmt.Printf("%s\n", string(b3))
}
