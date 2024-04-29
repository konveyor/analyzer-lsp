FROM golang:1.20 as go-builder

COPY / /analyzer-lsp

WORKDIR /generic-external-provider

COPY external-providers/generic-external-provider/go.mod go.mod
COPY external-providers/generic-external-provider/go.sum go.sum

COPY external-providers/generic-external-provider/main.go main.go
COPY external-providers/generic-external-provider/pkg/ pkg/

RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN go build -o generic-external-provider main.go

FROM quay.io/konveyor/golang-dependency-provider as go-dep-provider

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV NODEJS_VERSION=18
RUN echo -e "[nodejs]\nname=nodejs\nstream=${NODEJS_VERSION}\nprofiles=\nstate=enabled\n" > /etc/dnf/modules.d/nodejs.module
RUN microdnf install gcc-c++ python-devel go-toolset python3-devel nodejs -y && \
    microdnf clean all && \
    rm -rf /var/cache/dnf
RUN python3 -m ensurepip --upgrade
RUN python3 -m pip install 'python-lsp-server>=1.8.2'
RUN npm install -g typescript-language-server typescript
RUN go install golang.org/x/tools/gopls@latest

COPY --from=go-builder /generic-external-provider/generic-external-provider /usr/local/bin/generic-external-provider
COPY --from=go-dep-provider /usr/local/bin/golang-dependency-provider /usr/local/bin/golang-dependency-provider

ENTRYPOINT [ "/usr/local/bin/generic-external-provider" ]
