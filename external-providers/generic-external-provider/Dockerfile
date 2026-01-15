ARG GOLANG_DEP_IMAGE=quay.io/konveyor/golang-dependency-provider:latest
FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as go-builder

USER 0

COPY / /analyzer-lsp

RUN mkdir generic-provider
RUN mkdir -p go/bin
COPY --chmod=0777 external-providers/generic-external-provider/go.mod generic-provider/go.mod
COPY external-providers/generic-external-provider/go.sum generic-provider/go.sum

COPY external-providers/generic-external-provider/main.go generic-provider/main.go
COPY external-providers/generic-external-provider/pkg/ generic-provider/pkg/

RUN mkdir -p /opt/app-root/src/go && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 generic-provider && \
    chmod -R g=u generic-provider

USER 0

# Fix cache ownership if it was populated by previous builds
RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd generic-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd generic-provider && go build -o generic-external-provider main.go

FROM ${GOLANG_DEP_IMAGE} as go-dep-provider

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV NODEJS_VERSION=18
RUN echo -e "[nodejs]\nname=nodejs\nstream=${NODEJS_VERSION}\nprofiles=\nstate=enabled\n" > /etc/dnf/modules.d/nodejs.module
RUN microdnf install gcc-c++ python-devel go-toolset python3-devel nodejs -y && \
    microdnf clean all && \
    rm -rf /var/cache/dnf
RUN python3 -m ensurepip --upgrade
RUN python3 -m pip install 'python-lsp-server>=1.8.2'
RUN npm install -g typescript-language-server typescript

RUN GOBIN=/usr/local/bin go install golang.org/x/tools/gopls@v0.20.0

COPY --from=go-builder /opt/app-root/src/generic-provider/generic-external-provider /usr/local/bin/generic-external-provider
COPY --from=go-dep-provider /usr/local/bin/golang-dependency-provider /usr/local/bin/golang-dependency-provider
COPY external-providers/generic-external-provider/entrypoint.sh /usr/local/bin/entrypoint.sh


ENTRYPOINT ["/usr/local/bin/generic-external-provider"]
