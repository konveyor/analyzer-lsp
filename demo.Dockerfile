FROM quay.io/konveyor/analyzer-lsp

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples

RUN python3 -m venv /analyzer-lsp/examples/python/.venv
RUN yes | python3 -m pip install -r /analyzer-lsp/examples/python/requirements.txt
RUN chmod +x /usr/bin/yq-external-provider

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer --enable-jaeger && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]
# ENTRYPOINT ["sh", "-c", "ls -l /usr/bin/yq-external-provider"]
