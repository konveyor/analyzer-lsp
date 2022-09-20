package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/shawn-hurley/jsonrpc-golang/lsp/protocol"
	openshiftrp "github.com/shawn-hurley/jsonrpc-golang/openshift-rp"
	rules "github.com/shawn-hurley/jsonrpc-golang/rules_bak"
)

//TODO(shawn-hurley) - this package/type name stutters.
var workspaceRules = []rules.Rule{
	{
		ImportRule: &rules.ImportRule{
			GoImportRule: &rules.GoImportRule{
				Import: "pkg/apis/apiextensions/v1beta1.CustomResourceDefinition",
				// TODO(shawn-hurley) - copy the windup ability to intersparse known text here.
				Message: "Use of deprecated and removed API",
			},
		},
	},
	{
		ImportRule: &rules.ImportRule{
			JavaImportRule: &rules.JavaImportRule{
				Import:  "io.fabric8.kubernetes.api.model.apiextensions.v1beta1.CustomResourceDefinition",
				Message: "Use of deprecated and removed API",
			},
		},
	},
}

func main() {
	ctx := context.Background()

	// Create a Golang Provider and a Java Provider

	golangProvider := openshiftrp.NewGolangProvider(
		openshiftrp.LSPConfig{
			Location: "/home/shurley/repos/jsonrpc-golang/examples/golang",
		},
		openshiftrp.ProviderConfig{},
	)

	javaProvider := openshiftrp.NewJavaProvider(
		openshiftrp.LSPConfig{
			Location: "/home/shurley/repos/jsonrpc-golang/examples/java",
			InitializationOptions: map[string]interface{}{
				"bundles": []string{"/home/shurley/repos/java-rule-addon/java-rule-addon.core/target/java-rule-addon.core-1.0.0-SNAPSHOT.jar"},
			},
		},
		openshiftrp.ProviderConfig{},
		"/home/shurley/repos/jdt-lang-server/workspace",
	)

	golangProvider.Connect(ctx)
	javaProvider.Connect(ctx)

	for _, r := range workspaceRules {
		if r.GoImportRule != nil {
			symbols := golangProvider.GetAllSymbols(r.GoImportRule.Import)
			foundRefs := map[string]interface{}{}
			for _, s := range symbols {
				if s.Kind == protocol.Struct {
					references := golangProvider.GetAllReferences(s)
					for _, ref := range references {
						if strings.Contains(ref.URI, "/home/shurley/repos/jsonrpc-golang/examples/golang") {
							foundRefs[fmt.Sprintf("location %v: %v", ref.URI, ref.Range.Start.Line)] = nil
						}
					}
				}
			}
			fmt.Printf("\nGolang rule violations:")
			for k := range foundRefs {
				fmt.Printf("\n%v\n%v\n", r.GoImportRule.Message, k)
			}
		}
		if r.JavaImportRule != nil {
			symbols := javaProvider.GetAllSymbols(r.JavaImportRule.Import)
			foundRefs := map[string]interface{}{}
			for _, s := range symbols {
				// For Java, we have to look at the names for now
				if strings.Contains(s.Name, r.JavaImportRule.Import) {
					// Java references only searchs in the project
					references := javaProvider.GetAllReferences(s)
					for _, ref := range references {
						if strings.Contains(ref.URI, "/home/shurley/repos/jsonrpc-golang/examples/java") {
							foundRefs[fmt.Sprintf("location %v: %v", ref.URI, ref.Range.Start.Line)] = nil
						}
					}
				}
			}
			fmt.Printf("\nJava rule violations:")
			for k := range foundRefs {
				fmt.Printf("\n%v\n%v\n", r.JavaImportRule.Message, k)
			}
		}
	}
}
