package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/bombsimon/logrusr/v3"
	"github.com/konveyor/analyzer-lsp/provider"
	"github.com/konveyor/generic-external-provider/pkg/generic"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

var (
	port = flag.Int("port", 0, "Port must be set")
)

// For debugging
type EvaluateCall struct {
	ServiceClient provider.ServiceClient
	Cap           string
	ConditionInfo []byte
}

// For debugging
func mainDebug() {
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog)

	ctx := context.TODO()
	client := generic.NewGenericProvider()

	golangSC, err := client.Init(ctx, log, provider.InitConfig{
		Location: "/home/jonah/Projects/analyzer-lsp/examples/golang",
		// WorkspaceFolders: []string{
		// 	"/home/jonah/Projects/analyzer-lsp/examples/golang",
		// },
		AnalysisMode: "full",
		ProviderSpecificConfig: map[string]interface{}{
			"name":          "go",
			"lspServerPath": "gopls",
			"lspArgs":       []interface{}{"-vv", "-logfile", "debug-go.log", "-rpc.trace"},
		},
	})
	_ = golangSC

	pythonSC, err := client.Init(ctx, log, provider.InitConfig{
		Location: "/home/jonah/Projects/analyzer-lsp/examples/python",
		// WorkspaceFolders: []string{
		// 	"/home/jonah/Projects/analyzer-lsp/examples/python",
		// },
		AnalysisMode: "full",
		ProviderSpecificConfig: map[string]interface{}{
			"name":          "python",
			"lspServerPath": "/home/jonah/Projects/analyzer-lsp/examples/python/.venv/bin/pylsp",
			"lspArgs":       []interface{}{"--log-file", "debug-python.log"},
			// Would like to use WorkspaceFolders and DependencyFolders instead of this
			"referencedOutputIgnoreContains": []string{
				"file:///home/jonah/Projects/analyzer-lsp/examples/python/__pycache__",
				"file:///home/jonah/Projects/analyzer-lsp/examples/python/.venv",
			},
		},
	})
	_ = pythonSC

	if err != nil {
		panic(err)
	}

	var calls []EvaluateCall
	calls = append(
		calls,
		EvaluateCall{pythonSC, "referenced", []byte(`{"referenced":{pattern: "hello_world"}}`)},
		EvaluateCall{pythonSC, "referenced", []byte(`{"referenced":{pattern: "speak"}}`)},
		EvaluateCall{pythonSC, "referenced", []byte(`{"referenced":{pattern: "create_custom_resource_definition"}}`)},
		EvaluateCall{golangSC, "referenced", []byte(`{"referenced":{pattern: "v1beta1.CustomResourceDefinition"}}`)},
		EvaluateCall{golangSC, "referenced", []byte(`{"referenced":{pattern: "HelloWorld"}}`)},
	)

	var responses []provider.ProviderEvaluateResponse

	for _, call := range calls {
		response, err := call.ServiceClient.Evaluate(ctx, call.Cap, call.ConditionInfo)
		if err != nil {
			panic(err)
		}

		responses = append(responses, response)

		fmt.Printf("Service Client: %s\n", call.ServiceClient.(*generic.GenericServiceClient).Config.ProviderSpecificConfig["name"])
		fmt.Printf("Evaluated: %s, %s\n", call.Cap, string(call.ConditionInfo))
		fmt.Printf("Incidents:\n")
		b, _ := yaml.Marshal(response.Incidents)
		s := string(b)
		fmt.Printf("%s\n", s)

		fmt.Printf("Matched: %v\n", response.Matched)
		fmt.Printf("TemplateContext: %v\n", response.TemplateContext)
	}
}

func main() {
	manualDebug := false
	if manualDebug {
		mainDebug()
		return
	}

	flag.Parse()
	logrusLog := logrus.New()
	logrusLog.SetOutput(os.Stdout)
	logrusLog.SetFormatter(&logrus.TextFormatter{})
	// need to do research on mapping in logrusr to level here TODO
	logrusLog.SetLevel(logrus.Level(5))

	log := logrusr.New(logrusLog)

	client := generic.NewGenericProvider()

	if port == nil || *port == 0 {
		panic(fmt.Errorf("must pass in the port for the external provider"))
	}

	s := provider.NewServer(client, *port, log)
	ctx := context.TODO()
	s.Start(ctx)
}
