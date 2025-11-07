FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as go-builder

copy / /analyzer-lsp

RUN mkdir golang-dependency
COPY --chmod=0777 external-providers/golang-dependency-provider/go.mod golang-dependency/go.mod
COPY external-providers/golang-dependency-provider/go.sum golang-dependency/go.sum

COPY external-providers/golang-dependency-provider/main.go golang-dependency/main.go

RUN --mount=type=cache,id=gomod,uid=1001,target=go/pkg/mod cd golang-dependency && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,target=go/pkg/mod cd golang-dependency && go build -o golang-dependency-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /opt/app-root/src/golang-dependency/golang-dependency-provider /usr/local/bin/golang-dependency-provider

ENTRYPOINT [ "/usr/local/bin/golang-dependency-provider" ]
