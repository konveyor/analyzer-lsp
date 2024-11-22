FROM golang:1.21 as go-builder

copy / /analyzer-lsp

WORKDIR /yq-external-provider

COPY external-providers/yq-external-provider/go.mod go.mod
COPY external-providers/yq-external-provider/go.sum go.sum

COPY external-providers/yq-external-provider/main.go main.go
COPY external-providers/yq-external-provider/pkg/ pkg/

RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN go build -o yq-external-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y wget tar xz gzip && \
    microdnf clean all
ARG TARGETARCH
ARG YQ_VERSION="v4.40.5"
ARG YQ_BINARY="yq_linux_${TARGETARCH}"
RUN wget "https://github.com/mikefarah/yq/releases/download/${YQ_VERSION}/${YQ_BINARY}.tar.gz" -O - | tar xz && \
    mv ${YQ_BINARY} /usr/local/bin/yq

COPY --from=go-builder /yq-external-provider/yq-external-provider /usr/local/bin/yq-external-provider

ENTRYPOINT [ "/usr/local/bin/yq-external-provider" ]
