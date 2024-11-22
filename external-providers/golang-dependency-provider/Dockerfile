FROM registry.access.redhat.com/ubi9/go-toolset:1.21 as go-builder

copy / /analyzer-lsp

WORKDIR /go-dependency-provider

COPY external-providers/golang-dependency-provider/go.mod go.mod
COPY external-providers/golang-dependency-provider/go.sum go.sum

COPY external-providers/golang-dependency-provider/main.go main.go

RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN go build -o golang-dependency-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /go-dependency-provider/golang-dependency-provider /usr/local/bin/golang-dependency-provider

ENTRYPOINT [ "/usr/local/bin/golang-dependency-provider" ]
