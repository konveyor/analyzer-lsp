package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/konveyor/analyzer-lsp/provider/java"
	"github.com/konveyor/analyzer-lsp/provider/lib"
	"gopkg.in/yaml.v2"
)

var (
	outputFile = flag.String("output", "output.yaml", "path to output file")
)

func main() {
	flag.Parse()

	p := java.NewJavaProvider(lib.Config{Location: "../examples/java"})
	if !p.HasCapability("dependency") {
		fmt.Println("Provider does not have dependency capability")
		return
	}
	deps, err := p.GetDependencies()

	if err != nil {
		panic(err)
	}

	// Write results out to CLI
	b, _ := yaml.Marshal(deps)
	if len(deps) > 0 && err == nil {
		fmt.Printf("%s", string(b))
	}

	os.WriteFile(*outputFile, b, 0644)
}
