FROM debian:buster AS jaeger-builder

WORKDIR /jaeger

RUN apt-get update && \
    apt-get install -y curl && \
    curl -L -o jaeger-1.47.0-linux-amd64.tar.gz https://github.com/jaegertracing/jaeger/releases/download/v1.47.0/jaeger-1.47.0-linux-amd64.tar.gz && \
    tar -xzf jaeger-1.47.0-linux-amd64.tar.gz && \
    rm jaeger-1.47.0-linux-amd64.tar.gz


FROM quay.io/konveyor/analyzer-lsp AS analyzer-builder

COPY --from=jaeger-builder /jaeger/jaeger-1.47.0-linux-amd64/* /usr/local/bin/

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY open-source-libs.txt /analyzer-lsp/open-source-libs.txt

EXPOSE 5775/udp 6831/udp 6832/udp 5778 16686 14268 9411

ENTRYPOINT ["sh", "-c", "jaeger-all-in-one & sleep 10 && konveyor-analyzer --enable-jaeger=true --jaeger-endpoint=http://localhost:14268/api/traces && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]