ARG GOLANG_DEP_IMAGE=quay.io/konveyor/golang-dependency-provider:latest

# Go gRPC provider binary image with gopls + golang-dependency-provider runtime.
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as go-builder

USER 0

RUN mkdir go-provider
COPY / /analyzer-lsp

COPY --chmod=0777 external-providers/go-external-provider/go.mod go-provider/go.mod
COPY external-providers/go-external-provider/go.sum go-provider/go.sum

COPY external-providers/go-external-provider/main.go go-provider/main.go
COPY external-providers/go-external-provider/pkg/ go-provider/pkg/

RUN mkdir -p /opt/app-root/src/go && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 go-provider && \
    chmod -R g=u go-provider

USER 0

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd go-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd go-provider && go build -o go-external-provider main.go

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod GOBIN=/opt/app-root/src/go-provider go install golang.org/x/tools/gopls@v0.20.0

FROM ${GOLANG_DEP_IMAGE} as go-dep-provider

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /opt/app-root/src/go-provider/go-external-provider /usr/local/bin/go-external-provider
COPY --from=go-builder /opt/app-root/src/go-provider/gopls /usr/local/bin/gopls
COPY --from=go-dep-provider /usr/local/bin/golang-dependency-provider /usr/local/bin/golang-dependency-provider
WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001
ENV HOME /addon

ENTRYPOINT ["/usr/local/bin/go-external-provider"]
