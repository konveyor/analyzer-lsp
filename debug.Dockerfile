FROM golang:1.19 as builder
WORKDIR /analyzer-lsp

COPY cmd /analyzer-lsp/cmd
COPY engine /analyzer-lsp/engine
COPY event /analyzer-lsp/event
COPY output /analyzer-lsp/output
COPY jsonrpc2 /analyzer-lsp/jsonrpc2
COPY jsonrpc2_v2 /analyzer-lsp/jsonrpc2_v2
COPY lsp /analyzer-lsp/lsp
COPY parser /analyzer-lsp/parser
COPY provider /analyzer-lsp/provider
COPY tracing /analyzer-lsp/tracing
COPY external-providers /analyzer-lsp/external-providers
COPY go.mod /analyzer-lsp/go.mod
COPY go.sum /analyzer-lsp/go.sum
COPY Makefile /analyzer-lsp/Makefile

RUN go install github.com/go-delve/delve/cmd/dlv@latest

RUN go build -gcflags="all=-N -l" -o konveyor-analyzer ./cmd/analyzer/main.go
RUN go build -gcflags="all=-N -l" -o konveyor-analyzer-dep ./cmd/dep/main.go
RUN cd external-providers/generic-external-provider  && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -gcflags="all=-N -l" -o generic-external-provider main.go
RUN cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -gcflags="all=-N -l" -o golang-dependency-provider main.go
RUN cd external-providers/yq-external-provider       && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -gcflags="all=-N -l" -o yq-external-provider main.go
RUN cd external-providers/java-external-provider     && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && go mod tidy && go build -gcflags="all=-N -l" -o java-external-provider main.go


FROM registry.access.redhat.com/ubi9/ubi-minimal:latest as yq-builder
RUN microdnf install -y wget tar xz gzip && \
    microdnf clean all
ARG TARGETARCH
ARG YQ_VERSION="v4.40.5"
ARG YQ_BINARY="yq_linux_${TARGETARCH}"
RUN wget "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}.tar.gz" -O - | tar xz && \
    mv ${YQ_BINARY} /usr/bin/yq

FROM jaegertracing/all-in-one:latest AS jaeger-builder

FROM quay.io/konveyor/jdtls-server-base

RUN microdnf install gcc-c++ python-devel python3-devel -y
RUN python3 -m ensurepip --upgrade
RUN python3 -m pip install 'python-lsp-server>=1.8.2'

ENV NODEJS_VERSION=18
RUN echo -e "[nodejs]\nname=nodejs\nstream=${NODEJS_VERSION}\nprofiles=\nstate=enabled\n" > /etc/dnf/modules.d/nodejs.module
RUN microdnf install nodejs -y
RUN npm install -g typescript-language-server typescript

COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/local/bin/all-in-one-linux
COPY --from=yq-builder /usr/bin/yq /usr/local/bin/yq
COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/local/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/konveyor-analyzer-dep /usr/local/bin/konveyor-analyzer-dep
COPY --from=builder /analyzer-lsp/external-providers/generic-external-provider/generic-external-provider /usr/local/bin/generic-external-provider
COPY --from=builder /analyzer-lsp/external-providers/yq-external-provider/yq-external-provider /usr/local/bin/yq-external-provider
COPY --from=builder /analyzer-lsp/external-providers/golang-dependency-provider/golang-dependency-provider /usr/local/bin/golang-dependency-provider
COPY --from=builder /analyzer-lsp/external-providers/java-external-provider/java-external-provider /usr/local/bin/java-external-provider

COPY --from=builder /go/bin/dlv /

COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

EXPOSE 16686 40000

ENTRYPOINT ["/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec"]
CMD ["/usr/local/bin/konveyor-analyzer"]
