IMG_ANALYZER ?= konveyor-analyzer-local
TAG_JAVA_BUNDLE ?= latest
IMG_JAVA_PROVIDER ?= java-provider
IMG_DOTNET_PROVIDER ?= dotnet-provider
IMG_GENERIC_PROVIDER ?= generic-provider
IMG_GO_DEP_PROVIDER ?= golang-dep-provider
IMG_YQ_PROVIDER ?= yq-provider
OS := $(shell uname -s)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

ifeq ($(OS),Linux)
	MOUNT_OPT := :z
else
	MOUNT_OPT :=
endif

build-dir:
	mkdir -p build

build: build-dir analyzer deps golang-dependency-provider external-generic yq-external-provider java-external-provider dotnet-external-provider

analyzer: build-dir
	go build -o build/konveyor-analyzer ./cmd/analyzer/main.go
	if [ "${GOOS}" == "windows" ]; then mv build/konveyor-analyzer build/konveyor-analyzer.exe; fi

external-generic: build-dir
	(cd external-providers/generic-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o ../../build/generic-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/generic-external-provider build/generic-external-provider.exe; fi

golang-dependency-provider: build-dir
	(cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o ../../build/golang-dependency-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/golang-dependency-provider build/golang-dependency-provider.exe; fi

yq-external-provider: build-dir
	(cd external-providers/yq-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o ../../build/yq-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/yq-external-provider build/yq-external-provider.exe; fi

java-external-provider: build-dir
	(cd external-providers/java-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o ../../build/java-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/java-external-provider build/java-external-provider.exe; fi

dotnet-external-provider: build-dir
	(cd external-providers/dotnet-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.23.9 && go build -o ../../build/dotnet-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/dotnet-external-provider build/dotnet-external-provider.exe; fi

deps: build-dir
	go build -o build/konveyor-analyzer-dep ./cmd/dep/main.go
	if [ "${GOOS}" == "windows" ]; then mv build/konveyor-analyzer-dep build/konveyor-analyzer-dep.exe; fi

image-build:
	docker build --build-arg=JAVA_BUNDLE_TAG=$(TAG_JAVA_BUNDLE) -f Dockerfile . -t $(IMG_ANALYZER)

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

run-demo-image:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) -v $(PWD)/demo-dep-output.yaml:/analyzer-lsp/demo-dep-output.yaml:Z -v $(PWD)/demo-output.yaml:/analyzer-lsp/output.yaml:Z $(IMG_ANALYZER) --output-file=/analyzer-lsp/output.yaml --dep-output-file=/analyzer-lsp/demo-dep-output.yaml --dep-label-selector='!konveyor.io/dep-source=open-source'

# Provider-specific test targets
run-java-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/external-providers/java-external-provider/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-java
	podman run --pod analyzer-java --name java-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651

run-demo-java:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-java \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/java-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml:Z \
		-v $(PWD)/external-providers/java-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json:Z \
		-v $(PWD)/external-providers/java-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json

stop-java-provider-pod:
	podman pod kill analyzer-java || true
	podman pod rm analyzer-java || true
	podman volume rm test-data || true

run-generic-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-generic
	podman run --entrypoint /usr/local/bin/entrypoint.sh --pod analyzer-generic --name golang-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14653
	podman run --pod analyzer-generic --name nodejs -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14654 --name nodejs
	podman run --pod analyzer-generic --name python -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14655 --name pylsp

run-demo-generic:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-generic \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json

stop-generic-provider-pod:
	podman pod kill analyzer-generic || true
	podman pod rm analyzer-generic || true
	podman volume rm test-data || true

run-yaml-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-yaml
	podman run --pod analyzer-yaml --name yq -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14652

run-demo-yaml:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-yaml \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/yq-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml:Z \
		-v $(PWD)/external-providers/yq-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json:Z \
		-v $(PWD)/external-providers/yq-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json

stop-yaml-provider-pod:
	podman pod kill analyzer-yaml || true
	podman pod rm analyzer-yaml || true
	podman volume rm test-data || true

# Run all provider tests sequentially
test-all-providers: test-java test-generic test-yaml
	@echo "All provider tests completed successfully!"

test-java: build-java-provider run-java-provider-pod run-demo-java stop-java-provider-pod

test-generic: build-golang-dep-provider build-generic-provider run-generic-provider-pod run-demo-generic stop-generic-provider-pod

test-yaml: build-yq-provider run-yaml-provider-pod run-demo-yaml stop-yaml-provider-pod

stop-external-providers-pod: stop-external-providers
	podman pod kill analyzer
	podman pod rm analyzer
	podman volume rm test-data

extract-maven-index-files:
	podman run --name temp-jdtls -d quay.io/konveyor/jdtls-server-base:latest
	podman cp temp-jdtls:/usr/local/etc/maven-index.txt $(PWD)/external-providers/java-external-provider/pkg/java_external_provider/dependency/testdata/
	podman stop temp-jdtls || true
	podman rm temp-jdtls || true

run-index-benchmark:
	cd $(PWD)/external-providers/java-external-provider/pkg/java_external_provider/dependency/ &&  go test -bench=. -benchmem -benchtime=5s
