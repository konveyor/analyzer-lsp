# Build the manager binary
FROM golang:1.20 as builder

COPY / /analyzer-lsp

WORKDIR /java-provider

COPY external-providers/java-external-provider/go.mod go.mod
COPY external-providers/java-external-provider/go.sum go.sum


COPY external-providers/java-external-provider/main.go main.go
COPY external-providers/java-external-provider/pkg/ pkg/

RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -gcflags="all=-N -l" -a -o java-external-provider main.go

RUN go install github.com/go-delve/delve/cmd/dlv@latest

FROM quay.io/konveyor/jdtls-server-base:latest

COPY --from=builder /java-provider/java-external-provider /usr/local/bin/java-external-provider
COPY --from=builder /go/bin/dlv /usr/local/bin/dlv

EXPOSE 14651 40000

ENTRYPOINT ["/usr/local/bin/dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec"]
CMD ["--", "/usr/local/bin/java-external-provider", "--port", "14651"]
