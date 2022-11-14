DOCKER_IMAGE = test

build:
	go build -o konveyor-analyzer main.go

image-build:
	docker build -f Dockerfile . -t $(DOCKER_IMAGE)
