FROM jaegertracing/all-in-one:latest AS jaeger-builder

FROM quay.io/konveyor/analyzer-lsp

COPY --from=jaeger-builder /go/bin/all-in-one-linux /usr/bin/

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY open-source-libs.txt /analyzer-lsp/open-source-libs.txt

EXPOSE 5775/udp 6831/udp 6832/udp 5778 16686 14268 9411

ENTRYPOINT ["sh", "-c", "all-in-one-linux & sleep 5 && konveyor-analyzer --enable-jaeger=true --jaeger-endpoint=http://localhost:14268/api/traces && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]