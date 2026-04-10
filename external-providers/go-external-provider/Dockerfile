# Go gRPC provider binary image with gopls runtime.
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as go-builder

USER 0

RUN mkdir go-provider
COPY / /analyzer-lsp

COPY --chmod=0644 external-providers/go-external-provider/go.mod go-provider/go.mod
COPY external-providers/go-external-provider/go.sum go-provider/go.sum

COPY external-providers/go-external-provider/main.go go-provider/main.go
COPY external-providers/go-external-provider/pkg/ go-provider/pkg/

RUN mkdir -p /opt/app-root/src/go/pkg/sumdb && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 go-provider && \
    chmod -R g=u go-provider

# Build as root: non-root + BuildKit cache mounts (uid=1001) often yields permission denied in CI on both amd64 and arm64.
# Cache ids are image-specific (avoid clashing with other Dockerfiles). Module + sumdb caches are
# architecture-agnostic; compiled output uses GOCACHE (not mounted). Matrix CI usually runs amd64/arm64 on separate runners.
RUN --mount=type=cache,id=gomod-go-external-provider,target=/opt/app-root/src/go/pkg/mod \
    --mount=type=cache,id=gosumdb-go-external-provider,target=/opt/app-root/src/go/pkg/sumdb \
    cd go-provider && \
    go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && \
    go mod tidy && \
    go build -o go-external-provider main.go && \
    GOBIN=/opt/app-root/src/go-provider go install golang.org/x/tools/gopls@v0.20.0

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /opt/app-root/src/go-provider/go-external-provider /usr/local/bin/go-external-provider
COPY --from=go-builder /opt/app-root/src/go-provider/gopls /usr/local/bin/gopls
COPY --from=go-builder /usr/bin/go /usr/local/bin/go
COPY --from=go-builder /usr/lib/golang /usr/lib/golang
WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001
ENV HOME=/addon \
    GOROOT=/usr/lib/golang \
    GOPATH=/addon/go \
    GOCACHE=/addon/.cache/go-build \
    GOMODCACHE=/addon/go/pkg/mod

ENTRYPOINT ["/usr/local/bin/go-external-provider"]
