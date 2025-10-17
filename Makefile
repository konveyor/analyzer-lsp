DOCKER_IMAGE = test
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
	podman run --entrypoint /usr/local/bin/konveyor-analyzer --pod=analyzer -v test-data:/analyzer-lsp/examples$(MOUNT_OPT) -v $(PWD)/demo-dep-output.yaml:/analyzer-lsp/demo-dep-output.yaml:Z -v $(PWD)/demo-output.yaml:/analyzer-lsp/output.yaml:Z localhost/testing:latest --output-file=/analyzer-lsp/output.yaml --dep-output-file=/analyzer-lsp/demo-dep-output.yaml --dep-label-selector='!konveyor.io/dep-source=open-source'

stop-external-providers-pod: stop-external-providers
	podman pod kill analyzer
	podman pod rm analyzer
	podman volume rm test-data

extract-maven-index-files:
	podman run --name temp-jdtls -d quay.io/konveyor/jdtls-server-base:latest
	podman cp temp-jdtls:/usr/local/etc/maven-index.txt $(PWD)/external-providers/java-external-provider/pkg/java_external_provider/testdata/
	podman cp temp-jdtls:/usr/local/etc/maven-index.idx $(PWD)/external-providers/java-external-provider/pkg/java_external_provider/testdata/
	podman stop temp-jdtls || true
	podman rm temp-jdtls || true

run-index-benchmark:
	cd $(PWD)/external-providers/java-external-provider/pkg/java_external_provider/ &&  go test -bench=BenchmarkConstructArtifactFromSHA -benchmem -benchtime=5s
