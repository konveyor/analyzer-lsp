# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/shawn_hurley/jdtls-server 
COPY  ./ /analyzer-lsp
COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

RUN mv demo-output.yaml ../demo-output.yaml
RUN go install golang.org/x/tools/gopls@latest
RUN go run main.go

CMD [ "diff", "../demo-output.yaml", "violation_output.yaml" ]
