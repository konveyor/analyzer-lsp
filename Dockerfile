FROM golang:1.18 as builder
WORKDIR /analyzer-lsp

COPY  cmd /analyzer-lsp/cmd
COPY  engine /analyzer-lsp/engine
COPY  output /analyzer-lsp/output
COPY  jsonrpc2 /analyzer-lsp/jsonrpc2
COPY  lsp /analyzer-lsp/lsp
COPY  parser /analyzer-lsp/parser
COPY  provider /analyzer-lsp/provider
COPY  tracing /analyzer-lsp/tracing
COPY  external-providers /analyzer-lsp/external-providers
COPY  go.mod /analyzer-lsp/go.mod
COPY  go.sum /analyzer-lsp/go.sum
COPY  Makefile /analyzer-lsp/Makefile

RUN make build

FROM debian:buster AS jaeger-builder
WORKDIR /jaeger

RUN apt-get update && \
    apt-get install -y curl && \
    curl -L -o jaeger-1.47.0-linux-amd64.tar.gz https://github.com/jaegertracing/jaeger/releases/download/v1.47.0/jaeger-1.47.0-linux-amd64.tar.gz && \
    tar -xzf jaeger-1.47.0-linux-amd64.tar.gz && \
    rm jaeger-1.47.0-linux-amd64.tar.gz

# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/konveyor/jdtls-server-base

COPY --from=jaeger-builder /jaeger/jaeger-1.47.0-linux-amd64/* /usr/bin/

COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/konveyor-analyzer-dep /usr/bin/konveyor-analyzer-dep
COPY --from=builder /analyzer-lsp/external-providers/generic-external-provider/generic-external-provider /usr/bin/generic-external-provider
COPY --from=builder /analyzer-lsp/external-providers/golang-dependency-provider/golang-dependency-provider /usr/bin/golang-dependency-provider

COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

EXPOSE 5775/udp 6831/udp 6832/udp 5778 16686 14268 9411

ENTRYPOINT ["sh", "-c", "jaeger-all-in-one && konveyor-analyzer"]
