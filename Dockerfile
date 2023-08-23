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

FROM jaegertracing/all-in-one:latest AS jaeger-builder

# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/konveyor/jdtls-server-base

COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/bin/

COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/konveyor-analyzer-dep /usr/bin/konveyor-analyzer-dep
COPY --from=builder /analyzer-lsp/external-providers/generic-external-provider/generic-external-provider /usr/bin/generic-external-provider
COPY --from=builder /analyzer-lsp/external-providers/golang-dependency-provider/golang-dependency-provider /usr/bin/golang-dependency-provider

COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer"]
