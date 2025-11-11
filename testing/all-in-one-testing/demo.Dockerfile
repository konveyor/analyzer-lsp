ARG UBER_IMAGE=localhost/uber-analyzer
FROM ${UBER_IMAGE}

WORKDIR /analyzer-lsp

COPY testing/rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY external-providers/java-external-provider/examples /analyzer-lsp/examples

COPY testing/all-in-one-testing/provider_container_settings.json /analyzer-lsp/provider_settings.json

RUN python3 -m venv /analyzer-lsp/examples/python/.venv
RUN yes | python3 -m pip install -r /analyzer-lsp/examples/python/requirements.txt

RUN microdnf install go-toolset vim procps -y
RUN go install golang.org/x/tools/gopls@v0.16.2

EXPOSE 16686

ENTRYPOINT ["sh", "-c", "all-in-one-linux &> /dev/null & sleep 5 && konveyor-analyzer --enable-jaeger && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]
