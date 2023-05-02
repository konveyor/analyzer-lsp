FROM golang:1.18 as builder
WORKDIR /analyzer-lsp
# TODO limit to prevent unnecessary rebuilds
COPY  . /analyzer-lsp
RUN make build

# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/konveyor/jdtls-server-base

COPY --from=builder /analyzer-lsp/konveyor-analyzer /usr/bin/konveyor-analyzer
COPY --from=builder /analyzer-lsp/external-providers/golang-external-provider/golang-external-provider /usr/bin/golang-external-provider

COPY provider_container_settings.json /analyzer-lsp/provider_settings.json
COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY demo-output.yaml /demo-output.yaml
COPY examples /analyzer-lsp/examples

WORKDIR /analyzer-lsp
RUN /bin/bash -c "golang-external-provider & disown; konveyor-analyzer --output-file violation_output.yaml"

CMD [ "diff", "../demo-output.yaml", "violation_output.yaml" ]
