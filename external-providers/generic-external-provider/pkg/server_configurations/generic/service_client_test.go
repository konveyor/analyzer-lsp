package generic_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bombsimon/logrusr/v3"
	"github.com/go-logr/logr"
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

var log logr.Logger
var ctx context.Context
var examplesDir string

func TestMain(m *testing.M) {
	flag.Parse()

	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))

	log = logrusr.New(logrusLog)
	ctx = context.TODO()

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}

	examplesDir = filepath.Join(filepath.Dir(ex), "../../../../../examples")
	fmt.Println(examplesDir)

	code := m.Run()

	os.Exit(code)
}

func TestHopefullyNothingBroke(t *testing.T) {
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	logrusLog.SetLevel(logrus.Level(5))

	log = logrusr.New(logrusLog)
	prov := generic_external_provider.NewGenericProvider("generic", log)

	goplsExamples := "file://" + filepath.Join(examplesDir, "/golang")

	goplsSC, err := prov.Init(ctx, log, provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":    "generic",
			"lspServerPath":    "gopls",
			"lspServerArgs":    []interface{}{"-vv", "-logfile", "debug-go.log", "-rpc.trace"},
			"workspaceFolders": []interface{}{goplsExamples},
		},
	})
	_ = goplsSC

	if err != nil {
		fmt.Printf("%v\n", err)
		panic(err)
	}

	var res json.RawMessage

	p0 := protocol.WorkspaceSymbolParams{
		Query: "dummy.HelloWorld",
	}

	err = goplsSC.(*generic.GenericServiceClient).Conn.Call(ctx, "workspace/symbol", p0).Await(ctx, &res)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", string(res))

	var calls []EvaluateCall
	calls = append(
		calls,
		EvaluateCall{goplsSC, "referenced", []byte(`{"referenced":{pattern: "HelloWorld"}}`)},
		EvaluateCall{goplsSC, "echo", []byte(`{"echo":{input: "what's up!"}}`)},
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
