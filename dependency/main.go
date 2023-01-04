package main

import (
	"fmt"

	"github.com/konveyor/analyzer-lsp/dependency/provider/java"
)

func main() {
	p := java.GetDepProvider()
	deps, err := p.GetDependencies("/home/shurley/repos/analyzer-lsp/examples/java/pom.xml")
	if err != nil {
		panic(err)
	}
	fmt.Printf("%#v", deps)
}
