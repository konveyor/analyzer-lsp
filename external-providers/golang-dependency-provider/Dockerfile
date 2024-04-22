FROM golang:1.20 as go-builder

copy / /analyzer-lsp

WORKDIR /go-dependency-provider

COPY external-providers/golang-dependency-provider/go.mod go.mod
COPY external-providers/golang-dependency-provider/go.sum go.sum

COPY external-providers/golang-dependency-provider/main.go main.go
RUN go mod tidy
RUN go build -o go-dependency-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /go-dependency-provider/go-dependency-provider /usr/local/bin/go-dependency-provider

ENTRYPOINT [ "/usr/local/bin/go-dependency-provider" ]
