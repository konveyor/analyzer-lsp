DOCKER_IMAGE = test

build: analyzer deps external-generic golang-dependency-provider yq-external-provider java-external-provider

analyzer:
	go build -o konveyor-analyzer ./cmd/analyzer/main.go

external-generic:
	( cd external-providers/generic-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o generic-external-provider main.go)

golang-dependency-provider:
	( cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o golang-dependency-provider main.go)

yq-external-provider:
	( cd external-providers/yq-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o yq-external-provider main.go)

java-external-provider:
	( cd external-providers/java-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -o java-external-provider main.go)

deps:
	go build -o konveyor-analyzer-dep ./cmd/dep/main.go

image-build:
	docker build -f Dockerfile . -t $(DOCKER_IMAGE)

run-external: build-dotnet-provider build-generic-provider build-golang-dep-provider build-java-provider build-yq-provider run-images

build-dotnet-provider:
	cd external-providers/dotnet-external-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	podman build -f external-providers/dotnet-external-provider/Dockerfile -t dotnet-provider .

build-generic-provider:
	cd external-providers/generic-external-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	podman build -f external-providers/generic-external-provider/Dockerfile -t generic-provider .

build-golang-dep-provider:
	cd external-providers/golang-dependency-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	podman build -f external-providers/golang-dependency-provider/Dockerfile -t golang-dep-provider .

build-java-provider:
	cd external-providers/java-external-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	podman build -f external-providers/java-external-provider/Dockerfile -t java-provider .

build-yq-provider:
	cd external-providers/yq-external-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	podman build -f external-providers/yq-external-provider/Dockerfile -t yq-provider .

run-images:
	podman run --name java-provider -d -p 14651:14651 -v $(PWD)/external-providers/java-external-provider/examples:/examples java-provider --port 14651
	podman run --name yq -d -p 14652:14652 -v $(PWD)/examples:/examples yq-provider --port 14652
	podman run --name golang-provider -d -p 14653:14653 -v $(PWD)/examples:/examples generic-provider --port 14653
	podman run --name nodejs -d -p 14654:14654 -v $(PWD)/examples:/examples generic-provider --port 14654 --name nodejs
	podman run --name python -d -p 14655:14655 -v $(PWD)/examples:/examples generic-provider --port 14655 --name pylsp

stop-external-providers:
	podman kill java-provider || true
	podman kill  yq || true
	podman kill  golang-provider || true
	podman kill  nodejs || true
	podman kill  python || true
	podman rm java-provider || true
	podman rm yq || true
	podman rm golang-provider || true
	podman rm nodejs || true
	podman rm python || true
	cd external-providers/yq-external-provider/ && go mod edit --dropreplace=github.com/konveyor/analyzer-lsp
	cd external-providers/java-external-provider/ && go mod edit --dropreplace=github.com/konveyor/analyzer-lsp
	cd external-providers/golang-dependency-provider/ && go mod edit --dropreplace=github.com/konveyor/analyzer-lsp
	cd external-providers/generic-external-provider/ && go mod edit --dropreplace=github.com/konveyor/analyzer-lsp
	cd external-providers/dotnet-external-provider/ && go mod edit --dropreplace=github.com/konveyor/analyzer-lsp
