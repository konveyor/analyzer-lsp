package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/konveyor/analyzer-lsp/dependency/provider/java"
	"gopkg.in/yaml.v2"
)

var (
	outputFile = flag.String("output", "output.yaml", "path to output file")
)

func main() {
	flag.Parse()

	p := java.GetDepProvider()
	deps, err := p.GetDependencies("../examples/java/pom.xml")

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
