# The unofficial base image w/ jdtls and gopls installed
FROM quay.io/shawn_hurley/jdtls-server 
COPY  ./ /analyzer-lsp
COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

RUN go install golang.org/x/tools/gopls@latest
CMD ["/bin/bash", "-c", "go run main.go;cat output.yaml"]

