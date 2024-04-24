DOCKER_IMAGE = test

build: analyzer deps golang-dependency-provider external-generic yq-external-provider java-external-provider

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

build-external: build-dotnet-provider build-golang-dep-provider build-generic-provider build-java-provider build-yq-provider

build-dotnet-provider:
	cd external-providers/dotnet-external-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	podman build -f external-providers/dotnet-external-provider/Dockerfile -t dotnet-provider .

build-generic-provider:
	cd external-providers/generic-external-provider/ && go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp
	sed -i 's,quay.io/konveyor/golang-dependency-provider,golang-dep-provider,g' external-providers/generic-external-provider/Dockerfile
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

run-external-providers-local:
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

run-external-providers-pod:
	podman volume create test-data
	# copy data to test data volume
	podman run --rm -v test-data:/target -v $(PWD)/examples:/src/ --entrypoint=cp alpine -a /src/. /target/
	podman run --rm -v test-data:/target -v $(PWD)/external-providers/java-external-provider/examples:/src/ --entrypoint=cp alpine -a /src/. /target/
	# run pods w/ defined ports for the test volumes
	podman pod create --name=analyzer
	podman run --pod analyzer --name java-provider -d -v test-data:/analyzer-lsp/examples java-provider --port 14651
	podman run --pod analyzer --name yq -d -v test-data:/analyzer-lsp/examples yq-provider --port 14652
	podman run --pod analyzer --name golang-provider -d -v test-data:/analyzer-lsp/examples generic-provider --port 14653
	podman run --pod analyzer --name nodejs -d -v test-data:/analyzer-lsp/examples generic-provider --port 14654 --name nodejs
	podman run --pod analyzer --name python -d -v test-data:/analyzer-lsp/examples generic-provider --port 14655 --name pylsp
	podman build -f demo-local.Dockerfile -t localhost/testing:latest

run-demo-image:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer -v $(PWD)/demo-dep-output.yaml:/analyzer-lsp/demo-dep-output.yaml:Z -v $(PWD)/demo-output.yaml:/analyzer-lsp/output.yaml:Z localhost/testing:latest --dep-output-file=demo-dep-output.yaml

stop-external-providers-pod:
	podman pod kill analyzer
	podman pod rm analyzer
	podman volume rm test-data
