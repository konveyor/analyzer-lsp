DOCKER_IMAGE = test

build: analyzer deps external-golang

analyzer:
	go build -o konveyor-analyzer ./cmd/analyzer/main.go

external-golang:
	( cd external-providers/golang-external-provider && go build -o golang-external-provider main.go)

deps:
	go build -o konveyor-analyzer-dep ./cmd/dep/main.go

image-build:
	docker build -f Dockerfile . -t $(DOCKER_IMAGE)
