FROM quay.io/konveyor/jdtls-server-base
COPY  ./ /analyzer-lsp
COPY provider_container_settings.json /analyzer-lsp/provider_settings.json

WORKDIR /analyzer-lsp

# CMD [ "go", "run", "main.go", "--error-on-violation"]
CMD [ "go", "run", "main.go", "--rules", "demo-rules/local-storage.windup-rewrite.yaml" ]
