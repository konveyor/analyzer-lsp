ARG JAVA_BUNDLE_TAG=latest
FROM quay.io/konveyor/jdtls-server-base:${JAVA_BUNDLE_TAG} as base

# Build the manager binary
FROM golang:1.23.9 as builder

COPY / /analyzer-lsp

WORKDIR /java-provider

COPY external-providers/java-external-provider/go.mod go.mod
COPY external-providers/java-external-provider/go.sum go.sum


COPY external-providers/java-external-provider/main.go main.go
COPY external-providers/java-external-provider/pkg/ pkg/

RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN go build -a -o java-external-provider main.go

FROM base

COPY --from=builder /java-provider/java-external-provider /usr/local/bin/java-external-provider
RUN mkdir -p /root/.gradle
COPY external-providers/java-external-provider/gradle/build.gradle /usr/local/etc/task.gradle
COPY external-providers/java-external-provider/gradle/build-v9.gradle /usr/local/etc/task-v9.gradle

ENV HOME /addon
EXPOSE 14651
WORKDIR /addon
ENTRYPOINT ["java-external-provider", "--port", "14651"]
