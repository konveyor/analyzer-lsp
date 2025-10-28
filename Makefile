DOCKER_IMAGE = test
TAG_JAVA_BUNDLE ?= latest
IMG_JAVA_PROVIDER ?= java-provider
IMG_DOTNET_PROVIDER ?= dotnet-provider
IMG_GENERIC_PROVIDER ?= generic-provider
IMG_GO_DEP_PROVIDER ?= golang-dep-provider
IMG_YQ_PROVIDER ?= yq-provider
OS := $(shell uname -s)
ifeq ($(OS),Linux)
	MOUNT_OPT := :z
else 
	MOUNT_OPT := 
endif

build: analyzer deps golang-dependency-provider external-generic yq-external-provider java-external-provider

analyzer:
	go build -o konveyor-analyzer ./cmd/analyzer/main.go

external-generic:
	( cd external-providers/generic-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o generic-external-provider main.go)

golang-dependency-provider:
	( cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o golang-dependency-provider main.go)

yq-external-provider:
	( cd external-providers/yq-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o yq-external-provider main.go)

java-external-provider:
	( cd external-providers/java-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o java-external-provider main.go)

deps:
	go build -o konveyor-analyzer-dep ./cmd/dep/main.go

image-build:
	docker build --build-arg=JAVA_BUNDLE_TAG=$(TAG_JAVA_BUNDLE) -f Dockerfile . -t $(DOCKER_IMAGE)

build-external: build-dotnet-provider build-golang-dep-provider build-generic-provider build-java-provider build-yq-provider

build-dotnet-provider:
	podman build -f external-providers/dotnet-external-provider/Dockerfile -t $(IMG_DOTNET_PROVIDER) .

build-generic-provider:
	sed -i 's,quay.io/konveyor/golang-dependency-provider,golang-dep-provider,g' external-providers/generic-external-provider/Dockerfile
	podman build -f external-providers/generic-external-provider/Dockerfile -t $(IMG_GENERIC_PROVIDER) .

build-golang-dep-provider:
	podman build -f external-providers/golang-dependency-provider/Dockerfile -t $(IMG_GO_DEP_PROVIDER) .

build-java-provider:
	podman build --build-arg=JAVA_BUNDLE_TAG=$(TAG_JAVA_BUNDLE) -f external-providers/java-external-provider/Dockerfile -t $(IMG_JAVA_PROVIDER) .

build-yq-provider:
	podman build -f external-providers/yq-external-provider/Dockerfile -t $(IMG_YQ_PROVIDER) .

run-external-providers-local:
	podman run --name java-provider -d -p 14651:14651 -v $(PWD)/external-providers/java-external-provider/examples:/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651
	podman run --name yq -d -p 14652:14652 -v $(PWD)/examples:/examples $(IMG_YQ_PROVIDER)$(MOUNT_OPT) --port 14652
	podman run --entrypoint /usr/local/bin/entrypoint.sh --name golang-provider -d -p 14653:14653 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14653
	podman run --name nodejs -d -p 14654:14654 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14654 --name nodejs
	podman run --name python -d -p 14655:14655 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14655 --name pylsp

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
	sed -i 's,golang-dep-provider,quay.io/konveyor/golang-dependency-provider,g' external-providers/generic-external-provider/Dockerfile

run-external-providers-pod:
	podman volume create test-data
	# copy data to test data volume
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/external-providers/java-external-provider/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	# run pods w/ defined ports for the test volumes
	podman pod create --name=analyzer
	podman run --pod analyzer --name java-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651
	podman run --pod analyzer --name yq -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14652
	podman run --entrypoint /usr/local/bin/entrypoint.sh --pod analyzer --name golang-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14653
	podman run --pod analyzer --name nodejs -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14654 --name nodejs
	podman run --pod analyzer --name python -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14655 --name pylsp
	podman build -f demo-local.Dockerfile -t localhost/testing:latest

run-demo-image:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) -v $(PWD)/demo-dep-output.yaml:/analyzer-lsp/demo-dep-output.yaml:Z -v $(PWD)/demo-output.yaml:/analyzer-lsp/output.yaml:Z localhost/testing:latest --rules=/analyzer-lsp/rule-example.yaml --provider-settings=/analyzer-lsp/provider_settings.json --output-file=/analyzer-lsp/output.yaml --dep-output-file=/analyzer-lsp/demo-dep-output.yaml

stop-external-providers-pod: stop-external-providers
	podman pod kill analyzer
	podman pod rm analyzer
	podman volume rm test-data
