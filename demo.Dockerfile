FROM quay.io/konveyor/analyzer-lsp

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY examples/golang /analyzer-lsp/external-providers/golang-dependency-provider/golang-dependency-provider
COPY open-source-libs.txt /analyzer-lsp/open-source-libs.txt
