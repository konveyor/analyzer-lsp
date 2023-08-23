FROM quay.io/konveyor/analyzer-lsp

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer --enable-jaeger && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]
