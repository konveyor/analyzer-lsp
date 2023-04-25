DOCKER_IMAGE = test

build: analyzer deps

analyzer:
	go build -o konveyor-analyzer main.go

deps:
	go build -o konveyor-analyzer-dep ./dependency/main.go

image-build:
	docker build -f Dockerfile . -t $(DOCKER_IMAGE)
