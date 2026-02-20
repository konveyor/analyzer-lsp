FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as builder

USER 0
WORKDIR /analyzer-lsp

COPY cmd /analyzer-lsp/cmd
COPY engine /analyzer-lsp/engine
COPY event /analyzer-lsp/event
COPY output /analyzer-lsp/output
COPY jsonrpc2_v2 /analyzer-lsp/jsonrpc2_v2
COPY lsp /analyzer-lsp/lsp
COPY parser /analyzer-lsp/parser
COPY provider /analyzer-lsp/provider
COPY progress /analyzer-lsp/progress
COPY tracing /analyzer-lsp/tracing
COPY external-providers /analyzer-lsp/external-providers
COPY go.mod /analyzer-lsp/go.mod
COPY go.sum /analyzer-lsp/go.sum
COPY Makefile /analyzer-lsp/Makefile

RUN mkdir -p build /opt/app-root/src/go && \
    chgrp -R 0 /analyzer-lsp /opt/app-root/src/go && \
    chmod -R g=u /analyzer-lsp /opt/app-root/src/go

USER 0

# Fix cache ownership if it was populated by previous builds
RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod make analyzer deps

FROM jaegertracing/all-in-one:latest AS jaeger-builder

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/local/bin/all-in-one-linux
COPY --from=builder /analyzer-lsp/build/konveyor-analyzer /usr/local/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/build/konveyor-analyzer-dep /usr/local/bin/konveyor-analyzer-dep

WORKDIR /analyzer-lsp
RUN chgrp -R 0 /analyzer-lsp && chmod -R g=u /analyzer-lsp

EXPOSE 16686

USER 1001

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer"]
