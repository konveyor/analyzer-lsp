FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as go-builder

USER 0

RUN mkdir yq
copy / /analyzer-lsp

COPY --chmod=0777 external-providers/yq-external-provider/go.mod yq/go.mod
COPY external-providers/yq-external-provider/go.sum yq/go.sum

COPY external-providers/yq-external-provider/main.go yq/main.go
COPY external-providers/yq-external-provider/pkg/ yq/pkg/

RUN mkdir -p /opt/app-root/src/go && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 yq && \
    chmod -R g=u yq

USER 0

# Fix cache ownership if it was populated by previous builds
RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd yq && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd yq && go build -o yq-external-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y wget tar xz gzip && \
    microdnf clean all
ARG TARGETARCH
ARG YQ_VERSION="v4.40.5"
ARG YQ_BINARY="yq_linux_${TARGETARCH}"
RUN wget "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}.tar.gz" -O - | tar xz && \
    mv ${YQ_BINARY} /usr/local/bin/yq

COPY --from=go-builder /opt/app-root/src/yq/yq-external-provider /usr/local/bin/yq-external-provider

ENTRYPOINT [ "/usr/local/bin/yq-external-provider" ]

