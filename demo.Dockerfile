FROM quay.io/konveyor/analyzer-lsp

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY open-source-libs.txt /analyzer-lsp/open-source-libs.txt

EXPOSE 5775/udp 6831/udp 6832/udp 5778 16686 14268 9411

ENTRYPOINT ["sh", "-c", "all-in-one-linux & sleep 5 && konveyor-analyzer && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]