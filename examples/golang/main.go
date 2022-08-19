package main

import (
	"fmt"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

func main() {
	fmt.Println(v1beta1.CustomResourceDefinition{})
}
