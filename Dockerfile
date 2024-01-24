FROM golang:1.19 as builder
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

RUN microdnf install gcc-c++ python-devel python3-devel -y
RUN python3 -m ensurepip --upgrade
RUN python3 -m pip install python-lsp-server

COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/local/bin/all-in-one-linux

COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/konveyor-analyzer-dep /usr/bin/konveyor-analyzer-dep
COPY --from=builder /analyzer-lsp/external-providers/generic-external-provider/generic-external-provider /usr/bin/generic-external-provider
COPY --from=builder /analyzer-lsp/external-providers/golang-dependency-provider/golang-dependency-provider /usr/bin/golang-dependency-provider

COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp
RUN chgrp -R 0 /analyzer-lsp && chmod -R g=u /analyzer-lsp

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer"]
