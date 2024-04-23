#FROM quay.io/konveyor/analyzer-lsp
FROM 07aa1b90aad309e7117d422783c3ef34ade7da2bd94726dc0b8365199744eebe

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY external-providers/java-external-provider/examples /analyzer-lsp/examples

COPY provider_pod_local_settings.json /analyzer-lsp/provider_settings.json

RUN python3 -m venv /analyzer-lsp/examples/python/.venv
RUN yes | python3 -m pip install -r /analyzer-lsp/examples/python/requirements.txt

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer --enable-jaeger && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]
