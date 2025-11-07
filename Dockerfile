FROM registry.access.redhat.com/ubi9/ubi-minimal:latest as base

FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as builder

COPY cmd cmd
COPY engine engine
COPY  event event
COPY output output
COPY  jsonrpc2_v2 jsonrpc2_v2
COPY lsp lsp
COPY parser parser
COPY provider provider
COPY progress progress
COPY tracing tracing
COPY go.mod go.mod
COPY go.sum go.sum
COPY Makefile Makefile

RUN --mount=type=cache,id=gomod,uid=1001,target=go/pkg/mod make analyzer deps

FROM jaegertracing/all-in-one:latest AS jaeger-builder

FROM base


COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/local/bin/all-in-one-linux
COPY --from=builder /opt/app-root/src/build/konveyor-analyzer /usr/local/bin/konveyor-analyzer
COPY --from=builder /opt/app-root/src/build/konveyor-analyzer-dep /usr/local/bin/konveyor-analyzer-dep

WORKDIR /analyzer-lsp
RUN chgrp -R 0 /analyzer-lsp && chmod -R g=u /analyzer-lsp

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer"]
