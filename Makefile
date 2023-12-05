DOCKER_IMAGE = test

build: analyzer deps external-generic golang-dependency-provider

analyzer:
	go build -o konveyor-analyzer ./cmd/analyzer/main.go

external-generic:
	( cd external-providers/generic-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o generic-external-provider main.go)

golang-dependency-provider:
	( cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o golang-dependency-provider main.go)

yq-external-provider:
	( cd external-providers/yq-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o yq-external-provider main.go)

deps:
	go build -o konveyor-analyzer-dep ./cmd/dep/main.go

image-build:
	docker build -f Dockerfile . -t $(DOCKER_IMAGE)
