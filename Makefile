DOCKER_IMAGE = test

build: analyzer deps external-generic

analyzer:
	go build -o konveyor-analyzer ./cmd/analyzer/main.go

external-generic:
	( cd external-providers/generic-external-provider && go build -o generic-external-provider main.go)

deps:
	go build -o konveyor-analyzer-dep ./cmd/dep/main.go

image-build:
	docker build -f Dockerfile . -t $(DOCKER_IMAGE)
