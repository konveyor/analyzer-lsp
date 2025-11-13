FROM golang:1.23.9 as go-builder

copy / /analyzer-lsp

WORKDIR /go-dependency-provider

COPY external-providers/golang-dependency-provider/go.mod go.mod
COPY external-providers/golang-dependency-provider/go.sum go.sum

COPY external-providers/golang-dependency-provider/main.go main.go

RUN --mount=type=cache,id=gomod,uid=1001,target=go/pkg/mod cd golang-dependency && go build -o golang-dependency-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /go-dependency-provider/golang-dependency-provider /usr/local/bin/golang-dependency-provider

ENTRYPOINT [ "/usr/local/bin/golang-dependency-provider" ]
