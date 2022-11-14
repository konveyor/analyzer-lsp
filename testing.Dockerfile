# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/konveyor/jdtls-server-base
COPY  ./ /analyzer-lsp
COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

RUN mv demo-output.yaml ../demo-output.yaml
RUN go run main.go --output-file violation_output.yaml

CMD [ "diff", "../demo-output.yaml", "violation_output.yaml" ]