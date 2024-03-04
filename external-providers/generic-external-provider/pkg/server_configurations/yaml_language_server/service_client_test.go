package yaml_language_server_test

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/generic_external_provider"
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

	prov := generic_external_provider.NewGenericProvider("yaml_language_server")

	yamlSC, err := prov.Init(ctx, log, provider.InitConfig{
		ProviderSpecificConfig: map[string]interface{}{
			"lspServerName":    "yaml_language_server",
			"lspServerPath":    "node",
			"lspServerArgs":    []interface{}{"/home/jonah/Projects/yaml-language-server/out/server/src/server.js", "--stdio"},
			"workspaceFolders": []interface{}{"file:///home/jonah/Projects/analyzer-lsp/examples/yaml/"},
		},
	})
	_ = yamlSC

	if err != nil {
		panic(err)
	}

	var calls []EvaluateCall
	calls = append(
		calls,
		EvaluateCall{yamlSC, "referenced", []byte(`{"referenced":{pattern: "extensions/v1beta1"}}`)},
	)

	for _, call := range calls {
		response, err := call.ServiceClient.Evaluate(ctx, call.Cap, call.ConditionInfo)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Evaluated: %s, %s\n", call.Cap, string(call.ConditionInfo))
		fmt.Printf("Incidents:\n")
		b, _ := yaml.Marshal(response.Incidents)
		s := string(b)
		fmt.Printf("%s\n", s)

		fmt.Printf("Matched: %v\n", response.Matched)
		fmt.Printf("TemplateContext: %v\n", response.TemplateContext)
	}
}
