FROM golang:1.18 as builder
WORKDIR /analyzer-lsp
# TODO limit to prevent unnecessary rebuilds
COPY  . /analyzer-lsp
RUN make build

# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/konveyor/jdtls-server-base

WORKDIR /analyzer-lsp
# TODO limit to prevent unnecessary rebuilds
COPY  . /analyzer-lsp
COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/external-providers/golang-external-provider/golang-external-provider /usr/bin/golang-external-provider
COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

CMD ["/bin/bash", "-c", "konveyor-analyzer --output-file output.yaml; cat output.yaml"]
