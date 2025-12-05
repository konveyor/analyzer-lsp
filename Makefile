TAG ?= latest
IMG_ANALYZER ?= localhost/analyzer-lsp:$(TAG)
TAG_JAVA_BUNDLE ?= latest
IMG_JAVA_PROVIDER ?= localhost/java-provider:$(TAG)
IMG_GENERIC_PROVIDER ?= localhost/generic-provider:$(TAG)
IMG_GO_DEP_PROVIDER ?= localhost/golang-dep-provider:$(TAG)
IMG_YQ_PROVIDER ?= localhost/yq-provider:$(TAG)
IMG_C_SHARP_PROVIDER ?= quay.io/konveyor/c-sharp-provider:$(TAG)
OS := $(shell uname -s)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

MOUNT_OPT := :z

build-dir:
	mkdir -p build

build: build-dir analyzer deps golang-dependency-provider external-generic yq-external-provider java-external-provider

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

deps: build-dir
	go build -o build/konveyor-analyzer-dep ./cmd/dep/main.go
	if [ "${GOOS}" == "windows" ]; then mv build/konveyor-analyzer-dep build/konveyor-analyzer-dep.exe; fi

image-build:
	podman build -f Dockerfile . -t $(IMG_ANALYZER)

build-external:image-build build-generic-provider build-java-provider build-yq-provider

build-generic-provider: build-golang-dep-provider 
	podman build --build-arg GOLANG_DEP_IMAGE=$(IMG_GO_DEP_PROVIDER) -f external-providers/generic-external-provider/Dockerfile -t $(IMG_GENERIC_PROVIDER) .

build-golang-dep-provider:
	podman build -f external-providers/golang-dependency-provider/Dockerfile -t $(IMG_GO_DEP_PROVIDER) .

build-java-provider:
	podman build --build-arg=JAVA_BUNDLE_TAG=$(TAG_JAVA_BUNDLE) -f external-providers/java-external-provider/Dockerfile -t $(IMG_JAVA_PROVIDER) .

build-yq-provider:
	podman build -f external-providers/yq-external-provider/Dockerfile -t $(IMG_YQ_PROVIDER) .

run-external-providers-local:
	podman run --name java-provider -d -p 14651:14651 -v $(PWD)/external-providers/java-external-provider/examples:/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651
	podman run --name yq -d -p 14652:14652 -v $(PWD)/examples:/examples $(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14652
	podman run --entrypoint /usr/local/bin/entrypoint.sh --name golang-provider -d -p 14653:14653 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14653
	podman run --name nodejs -d -p 14654:14654 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14654 --name nodejs
	podman run --name python -d -p 14655:14655 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14655 --name pylsp

stop-external-providers:
	podman kill java-provider || true
	podman kill  yq || true
	podman kill  golang-provider || true
	podman kill  nodejs || true
	podman kill  python || true
	podman kill  c-sharp || true
	podman rm java-provider || true
	podman rm yq || true
	podman rm golang-provider || true
	podman rm nodejs || true
	podman rm python || true
	podman rm c-sharp || true

run-external-providers-pod:
	podman volume create test-data
	# copy data to test data volume
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/external-providers/java-external-provider/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	# run pods w/ defined ports for the test volumes
	podman pod create --name=analyzer
	podman run --pod analyzer --name java-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651
	podman run --pod analyzer --name yq -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14652
	podman run --pod analyzer --name c-sharp -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_C_SHARP_PROVIDER) --port 14656
	podman run --entrypoint /usr/local/bin/entrypoint.sh --pod analyzer --name golang-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14653
	podman run --pod analyzer --name nodejs -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14654 --name nodejs
	podman run --pod analyzer --name python -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14655 --name pylsp

run-demo-image:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) -v $(PWD)/demo-dep-output.yaml:/analyzer-lsp/demo-dep-output.yaml:Z -v $(PWD)/demo-output.yaml:/analyzer-lsp/output.yaml:Z -v $(PWD)/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z -v $(PWD)/provider_pod_local_settings.json:/analyzer-lsp/provider_settings.json:Z $(IMG_ANALYZER) --output-file=/analyzer-lsp/output.yaml --dep-output-file=/analyzer-lsp/demo-dep-output.yaml --dep-label-selector='!konveyor.io/dep-source=open-source' --rules=/analyzer-lsp/rule-example.yaml --provider-settings=/analyzer-lsp/provider_settings.json

# Provider-specific test targets
run-java-provider-pod:
	podman volume create test-data
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
		--provider-settings=/analyzer-lsp/provider_settings.json \
		--dep-label-selector='!konveyor.io/dep-source=open-source'

stop-java-provider-pod:
	podman pod kill analyzer-java || true
	podman pod rm analyzer-java || true
	podman volume rm test-data || true

run-generic-golang-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-generic-golang
	podman run --pod analyzer-generic-golang --name golang -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14651 --name generic
run-generic-python-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-generic-python
	podman run --pod analyzer-generic-python --name python -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14651 --name pylsp
run-generic-nodejs-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-generic-nodejs
	podman run --pod analyzer-generic-nodejs --name nodejs -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GENERIC_PROVIDER) --port 14651 --name nodejs

run-demo-generic-golang:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-generic-golang \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/golang-e2e/demo-output.yaml:/analyzer-lsp/output.yaml:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/golang-e2e/provider_settings.json:/analyzer-lsp/provider_settings.json:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/golang-e2e/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json
run-demo-generic-python:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-generic-python \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/python-e2e/demo-output.yaml:/analyzer-lsp/output.yaml:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/python-e2e/provider_settings.json:/analyzer-lsp/provider_settings.json:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/python-e2e/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json
run-demo-generic-nodejs:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-generic-nodejs \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/nodejs-e2e/demo-output.yaml:/analyzer-lsp/output.yaml:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/nodejs-e2e/provider_settings.json:/analyzer-lsp/provider_settings.json:Z \
		-v $(PWD)/external-providers/generic-external-provider/e2e-tests/nodejs-e2e/rule-example.yaml:/analyzer-lsp/rule-example.yaml:Z \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json

stop-generic-golang-provider-pod:
	podman pod kill analyzer-generic-golang || true
	podman pod rm analyzer-generic-golang || true
	podman volume rm test-data || true

stop-generic-python-provider-pod:
	podman pod kill analyzer-generic-python || true
	podman pod rm analyzer-generic-python || true
	podman volume rm test-data || true

stop-generic-nodejs-provider-pod:
	podman pod kill analyzer-generic-nodejs || true
	podman pod rm analyzer-generic-nodejs || true
	podman volume rm test-data || true

run-yaml-provider-pod:
	podman volume create test-data
	podman run --rm -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman pod create --name=analyzer-yaml
	podman run --pod analyzer-yaml --name yq -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14651

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


test-all: test-all-providers test-analyzer

test-analyzer: run-external-providers-pod run-demo-image stop-external-providers-pod

# Run all provider tests sequentially
test-all-providers: test-java test-generic test-yaml
	@echo "All provider tests completed successfully!"

test-java: run-java-provider-pod run-demo-java stop-java-provider-pod

test-golang: run-generic-golang-provider-pod run-demo-generic-golang stop-generic-golang-provider-pod

test-python: run-generic-python-provider-pod run-demo-generic-python stop-generic-python-provider-pod

test-nodejs: run-generic-nodejs-provider-pod run-demo-generic-nodejs stop-generic-nodejs-provider-pod

test-generic: test-nodejs test-python test-golang 

test-yaml: run-yaml-provider-pod run-demo-yaml stop-yaml-provider-pod

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
