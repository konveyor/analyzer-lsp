FROM golang:1.20 as go-builder

WORKDIR /yq-external-provider

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY main.go main.go
COPY pkg/ pkg/

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
