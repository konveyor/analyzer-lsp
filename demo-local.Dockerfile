FROM quay.io/konveyor/analyzer-lsp

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY external-providers/java-external-provider/examples /analyzer-lsp/examples

COPY provider_pod_local_settings.json /analyzer-lsp/provider_settings.json

RUN python3 -m venv /analyzer-lsp/examples/python/.venv
RUN yes | python3 -m pip install -r /analyzer-lsp/examples/python/requirements.txt
RUN cd /analyzer-lsp/examples/nodejs && npm install

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer --enable-jaeger && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]
