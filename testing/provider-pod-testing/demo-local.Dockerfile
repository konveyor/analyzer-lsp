FROM quay.io/konveyor/analyzer-lsp

WORKDIR /analyzer-lsp

COPY testing/rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY testing/provider-pod-testing/provider_pod_local_settings.json /analyzer-lsp/provider_settings.json

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer --enable-jaeger && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]
