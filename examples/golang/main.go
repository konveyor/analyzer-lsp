package main

import (
	"fmt"

	"github.com/konveyor/analyzer-lsp/examples/golang/dummy"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

func main() {
	fmt.Println(v1beta1.CustomResourceDefinition{})

	fmt.Println(dummy.HelloWorld())
}
