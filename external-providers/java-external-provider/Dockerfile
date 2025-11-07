ARG JAVA_BUNDLE_TAG=latest
FROM quay.io/konveyor/jdtls-server-base:${JAVA_BUNDLE_TAG} as base

# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as builder

copy / /analyzer-lsp

RUN mkdir java
COPY --chmod=0777 external-providers/java-external-provider/go.mod java/go.mod
COPY external-providers/java-external-provider/go.sum java/go.sum


COPY external-providers/java-external-provider/main.go java/main.go
COPY external-providers/java-external-provider/pkg/ java/pkg/

RUN --mount=type=cache,id=gomod,uid=1001,target=go/pkg/mod cd java && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,target=go/pkg/mod cd java && go build -a -o java-external-provider main.go

FROM base

COPY --from=builder /opt/app-root/src/java/java-external-provider /usr/local/bin/java-external-provider
RUN mkdir -p /root/.gradle
COPY external-providers/java-external-provider/gradle/build.gradle /usr/local/etc/task.gradle
COPY external-providers/java-external-provider/gradle/build-v9.gradle /usr/local/etc/task-v9.gradle

ENV HOME /addon
EXPOSE 14651
WORKDIR /addon
ENTRYPOINT ["java-external-provider", "--port", "14651"]
