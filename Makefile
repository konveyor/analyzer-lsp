USER_ID := $(shell id -u):0
TAG ?= latest
IMG_ANALYZER ?= localhost/analyzer-lsp:$(TAG)
TAG_JAVA_BUNDLE ?= latest
IMG_JAVA_PROVIDER ?= localhost/java-provider:$(TAG)
IMG_GENERIC_PROVIDER ?= localhost/generic-provider:$(TAG)
IMG_GO_PROVIDER ?= localhost/go-external-provider:$(TAG)
IMG_PYTHON_PROVIDER ?= localhost/python-external-provider:$(TAG)
IMG_NODE_PROVIDER ?= localhost/nodejs-external-provider:$(TAG)
IMG_GO_DEP_PROVIDER ?= localhost/golang-dep-provider:$(TAG)
IMG_YQ_PROVIDER ?= localhost/yq-provider:$(TAG)
IMG_C_SHARP_PROVIDER ?= quay.io/konveyor/c-sharp-provider:$(TAG)
OS := $(shell uname -s)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

MOUNT_OPT := :z

build-dir:
	mkdir -p build

build: build-dir analyzer deps golang-dependency-provider external-generic external-go-provider external-python-provider external-nodejs-provider yq-external-provider java-external-provider

analyzer: build-dir
	go build -o build/konveyor-analyzer ./cmd/analyzer/main.go
	if [ "${GOOS}" == "windows" ]; then mv build/konveyor-analyzer build/konveyor-analyzer.exe; fi

external-generic: build-dir
	(cd external-providers/generic-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/generic-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/generic-external-provider build/generic-external-provider.exe; fi

external-go-provider: build-dir
	(cd external-providers/go-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/go-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/go-external-provider build/go-external-provider.exe; fi

external-python-provider: build-dir
	(cd external-providers/python-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/python-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/python-external-provider build/python-external-provider.exe; fi

external-nodejs-provider: build-dir
	(cd external-providers/nodejs-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/nodejs-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/nodejs-external-provider build/nodejs-external-provider.exe; fi

golang-dependency-provider: build-dir
	(cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/golang-dependency-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/golang-dependency-provider build/golang-dependency-provider.exe; fi

yq-external-provider: build-dir
	(cd external-providers/yq-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/yq-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/yq-external-provider build/yq-external-provider.exe; fi

java-external-provider: build-dir
	(cd external-providers/java-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy -go=1.25 && go build -o ../../build/java-external-provider main.go)
	if [ "${GOOS}" == "windows" ]; then mv build/java-external-provider build/java-external-provider.exe; fi

deps: build-dir
	go build -o build/konveyor-analyzer-dep ./cmd/dep/main.go
	if [ "${GOOS}" == "windows" ]; then mv build/konveyor-analyzer-dep build/konveyor-analyzer-dep.exe; fi

image-build:
	podman build -f Dockerfile . -t $(IMG_ANALYZER)

build-external:image-build build-go-provider build-python-provider build-nodejs-provider build-java-provider build-yq-provider

build-generic-provider: build-golang-dep-provider 
	podman build --build-arg GOLANG_DEP_IMAGE=$(IMG_GO_DEP_PROVIDER) -f external-providers/generic-external-provider/Dockerfile -t $(IMG_GENERIC_PROVIDER) .

build-golang-dep-provider:
	podman build -f external-providers/golang-dependency-provider/Dockerfile -t $(IMG_GO_DEP_PROVIDER) .

build-go-provider: build-golang-dep-provider
	podman build --build-arg GOLANG_DEP_IMAGE=$(IMG_GO_DEP_PROVIDER) -f external-providers/go-external-provider/Dockerfile -t $(IMG_GO_PROVIDER) .

build-python-provider:
	podman build -f external-providers/python-external-provider/Dockerfile -t $(IMG_PYTHON_PROVIDER) .

build-nodejs-provider:
	podman build -f external-providers/nodejs-external-provider/Dockerfile -t $(IMG_NODE_PROVIDER) .

build-java-provider:
	podman build --build-arg=JAVA_BUNDLE_TAG=$(TAG_JAVA_BUNDLE) -f external-providers/java-external-provider/Dockerfile -t $(IMG_JAVA_PROVIDER) .

build-yq-provider:
	podman build -f external-providers/yq-external-provider/Dockerfile -t $(IMG_YQ_PROVIDER) .

run-external-providers-local:
	podman run --userns=keep-id --user=$(USER_ID) --name java-provider -d -p 14651:14651 -v $(PWD)/external-providers/java-external-provider/examples:/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651
	podman run --userns=keep-id --user=$(USER_ID) --name yq -d -p 14652:14652 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14652
	podman run --userns=keep-id --user=$(USER_ID) --name golang-provider -d -p 14653:14653 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_GO_PROVIDER) --port 14653 --name generic
	podman run --userns=keep-id --user=$(USER_ID) --name nodejs -d -p 14654:14654 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_NODE_PROVIDER) --port 14654 --name nodejs
	podman run --userns=keep-id --user=$(USER_ID) --name python -d -p 14655:14655 -v $(PWD)/examples:/examples$(MOUNT_OPT) $(IMG_PYTHON_PROVIDER) --port 14655 --name pylsp

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
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/external-providers/java-external-provider/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=0 -v test-data:/target$(MOUNT_OPT) --entrypoint=sh alpine -c 'chmod -R a+rwX /target'
	# run pods w/ defined ports for the test volumes
	podman pod create --name=analyzer --userns=keep-id
	podman run --pod analyzer --user=$(USER_ID) --name java-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651
	podman run --pod analyzer --user=$(USER_ID) --name yq -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14652
	podman run --pod analyzer --user=$(USER_ID) --name c-sharp -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_C_SHARP_PROVIDER) --port 14656
	podman run --pod analyzer --user=$(USER_ID) --name golang-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GO_PROVIDER) --port 14653 --name generic
	podman run --pod analyzer --user=$(USER_ID) --name nodejs -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_NODE_PROVIDER) --port 14654 --name nodejs
	podman run --pod analyzer --user=$(USER_ID) --name python -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_PYTHON_PROVIDER) --port 14655 --name pylsp

run-demo-image:
	podman run --user=$(USER_ID) --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) -v $(PWD)/demo-dep-output.yaml:/analyzer-lsp/demo-dep-output.yaml${MOUNT_OPT} -v $(PWD)/demo-output.yaml:/analyzer-lsp/output.yaml${MOUNT_OPT} -v $(PWD)/rule-example.yaml:/analyzer-lsp/rule-example.yaml${MOUNT_OPT} -v $(PWD)/provider_pod_local_settings.json:/analyzer-lsp/provider_settings.json${MOUNT_OPT} $(IMG_ANALYZER) --output-file=/analyzer-lsp/output.yaml --dep-output-file=/analyzer-lsp/demo-dep-output.yaml --dep-label-selector='!konveyor.io/dep-source=open-source' --rules=/analyzer-lsp/rule-example.yaml --provider-settings=/analyzer-lsp/provider_settings.json

# Provider-specific test targets
run-java-provider-pod:
	podman volume create test-data
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/external-providers/java-external-provider/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=0 -v test-data:/target$(MOUNT_OPT) --entrypoint=sh alpine -c 'chmod -R a+rwX /target'
	podman pod create --name=analyzer-java --userns=keep-id 
	podman run --pod analyzer-java --user=$(USER_ID) --name java-provider -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_JAVA_PROVIDER) --port 14651

run-demo-java:
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-java --user=$(USER_ID) \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/java-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml${MOUNT_OPT} \
		-v $(PWD)/external-providers/java-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json${MOUNT_OPT} \
		-v $(PWD)/external-providers/java-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml${MOUNT_OPT} \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json \
		--dep-label-selector='!konveyor.io/dep-source=open-source'

stop-java-provider-pod:
	podman pod kill analyzer-java || true
	podman pod rm analyzer-java || true
	podman volume rm test-data || true

run-go-provider-pod:
	podman volume create test-data
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=0 -v test-data:/target$(MOUNT_OPT) --entrypoint=sh alpine -c 'chmod -R a+rwX /target'
	podman pod create --name=analyzer-go --userns=keep-id 
	podman run --pod analyzer-go --user=$(USER_ID) --name golang -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_GO_PROVIDER) --port 14651 --name generic
run-python-provider-pod:
	podman volume create test-data
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=0 -v test-data:/target$(MOUNT_OPT) --entrypoint=sh alpine -c 'chmod -R a+rwX /target'
	podman pod create --name=analyzer-python --userns=keep-id
	podman run --pod analyzer-python --user=$(USER_ID) --name python -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_PYTHON_PROVIDER) --port 14651 --name pylsp
run-nodejs-provider-pod:
	podman volume create test-data
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=0 -v test-data:/target$(MOUNT_OPT) --entrypoint=sh alpine -c 'chmod -R a+rwX /target'
	podman pod create --name=analyzer-nodejs --userns=keep-id
	podman run --pod analyzer-nodejs --user=$(USER_ID) --name nodejs -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_NODE_PROVIDER) --port 14651 --name nodejs

run-demo-go:
	podman run --user=$(USER_ID) --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-go \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/go-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml${MOUNT_OPT} \
		-v $(PWD)/external-providers/go-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json${MOUNT_OPT} \
		-v $(PWD)/external-providers/go-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml${MOUNT_OPT} \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json \
		--dep-label-selector='!konveyor.io/dep-source=open-source'
run-demo-python:
	podman run --user=$(USER_ID) --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-python \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/python-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml${MOUNT_OPT} \
		-v $(PWD)/external-providers/python-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json${MOUNT_OPT} \
		-v $(PWD)/external-providers/python-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml${MOUNT_OPT} \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json \
		--dep-label-selector='!konveyor.io/dep-source=open-source'
run-demo-nodejs:
	podman run --user=$(USER_ID) --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-nodejs \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/nodejs-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml${MOUNT_OPT} \
		-v $(PWD)/external-providers/nodejs-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json${MOUNT_OPT} \
		-v $(PWD)/external-providers/nodejs-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml${MOUNT_OPT} \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json \
		--dep-label-selector='!konveyor.io/dep-source=open-source'

stop-go-provider-pod:
	podman pod kill analyzer-go || true
	podman pod rm analyzer-go || true
	podman volume rm test-data || true

stop-python-provider-pod:
	podman pod kill analyzer-python || true
	podman pod rm analyzer-python || true
	podman volume rm test-data || true

stop-nodejs-provider-pod:
	podman pod kill analyzer-nodejs || true
	podman pod rm analyzer-nodejs || true
	podman volume rm test-data || true

run-yaml-provider-pod:
	podman volume create test-data
	podman run --rm --user=$(USER_ID) -v test-data:/target$(MOUNT_OPT) -v $(PWD)/examples:/src/$(MOUNT_OPT) --entrypoint=cp alpine -a /src/. /target/
	podman run --rm --user=0 -v test-data:/target$(MOUNT_OPT) --entrypoint=sh alpine -c 'chmod -R a+rwX /target'
	podman pod create --name=analyzer-yaml --userns=keep-id 
	podman run --pod analyzer-yaml --user=$(USER_ID) --name yq -d -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) $(IMG_YQ_PROVIDER) --port 14651

run-demo-yaml:
	podman run --user=$(USER_ID) --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer-yaml \
		-v test-data:/analyzer-lsp/examples$(MOUNT_OPT) \
		-v $(PWD)/external-providers/yq-external-provider/e2e-tests/demo-output.yaml:/analyzer-lsp/output.yaml${MOUNT_OPT} \
		-v $(PWD)/external-providers/yq-external-provider/e2e-tests/provider_settings.json:/analyzer-lsp/provider_settings.json${MOUNT_OPT} \
		-v $(PWD)/external-providers/yq-external-provider/e2e-tests/rule-example.yaml:/analyzer-lsp/rule-example.yaml${MOUNT_OPT} \
		$(IMG_ANALYZER) \
		--output-file=/analyzer-lsp/output.yaml \
		--rules=/analyzer-lsp/rule-example.yaml \
		--provider-settings=/analyzer-lsp/provider_settings.json \
		--dep-label-selector='!konveyor.io/dep-source=open-source'

stop-yaml-provider-pod:
	podman pod kill analyzer-yaml || true
	podman pod rm analyzer-yaml || true
	podman volume rm test-data || true


test-all: test-all-providers test-analyzer

test-analyzer: run-external-providers-pod run-demo-image stop-external-providers-pod

# Run all provider tests sequentially
test-all-providers: test-java test-go test-python test-nodejs test-yaml
	@echo "All provider tests completed successfully!"

test-java: run-java-provider-pod run-demo-java stop-java-provider-pod

test-go: run-go-provider-pod run-demo-go stop-go-provider-pod

test-python: run-python-provider-pod run-demo-python stop-python-provider-pod

test-nodejs: run-nodejs-provider-pod run-demo-nodejs stop-nodejs-provider-pod

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
