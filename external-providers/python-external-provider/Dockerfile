# Python gRPC provider binary image with pylsp runtime.
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as go-builder

USER 0

RUN mkdir python-provider
COPY / /analyzer-lsp

COPY --chmod=0777 external-providers/python-external-provider/go.mod python-provider/go.mod
COPY external-providers/python-external-provider/go.sum python-provider/go.sum

COPY external-providers/python-external-provider/main.go python-provider/main.go
COPY external-providers/python-external-provider/pkg/ python-provider/pkg/

RUN mkdir -p /opt/app-root/src/go && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 python-provider && \
    chmod -R g=u python-provider

USER 0

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd python-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd python-provider && go build -o python-external-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install python3 python3-pip -y && \
    microdnf clean all && \
    rm -rf /var/cache/dnf
RUN python3 -m pip install --no-cache-dir 'python-lsp-server>=1.8.2'

COPY --from=go-builder /opt/app-root/src/python-provider/python-external-provider /usr/local/bin/python-external-provider
WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001
ENV HOME /addon

ENTRYPOINT ["/usr/local/bin/python-external-provider"]
