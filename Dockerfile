FROM golang:1.19 as builder
WORKDIR /analyzer-lsp

COPY cmd /analyzer-lsp/cmd
COPY engine /analyzer-lsp/engine
COPY output /analyzer-lsp/output
COPY jsonrpc2 /analyzer-lsp/jsonrpc2
COPY lsp /analyzer-lsp/lsp
COPY parser /analyzer-lsp/parser
COPY provider /analyzer-lsp/provider
COPY tracing /analyzer-lsp/tracing
COPY external-providers /analyzer-lsp/external-providers
COPY go.mod /analyzer-lsp/go.mod
COPY go.sum /analyzer-lsp/go.sum
COPY Makefile /analyzer-lsp/Makefile

RUN make build

# Add yq to the build stage
FROM docker.io/mikefarah/yq as yq-builder

# Continue with the rest of the Dockerfile
FROM jaegertracing/all-in-one:latest AS jaeger-builder

FROM quay.io/konveyor/jdtls-server-base

RUN microdnf install gcc-c++ python-devel python3-devel -y
RUN python3 -m ensurepip --upgrade
RUN python3 -m pip install python-lsp-server

COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/bin/

# Copy yq from the yq-builder stage to the final image
COPY --from=yq-builder /usr/bin/yq /usr/bin/yq

COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/konveyor-analyzer-dep /usr/bin/konveyor-analyzer-dep
COPY --from=builder /analyzer-lsp/external-providers/generic-external-provider/generic-external-provider /usr/bin/generic-external-provider
COPY --from=builder /analyzer-lsp/external-providers/yq-external-provider/yq-external-provider /usr/bin/yq-external-provider
COPY --from=builder /analyzer-lsp/external-providers/golang-dependency-provider/golang-dependency-provider /usr/bin/golang-dependency-provider

COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer"]
