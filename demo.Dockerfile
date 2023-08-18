FROM debian:buster AS jaeger-builder

WORKDIR /jaeger

RUN apt-get update && \
    apt-get install -y curl jq && \
    JAEGER_VERSION=$(curl -s https://api.github.com/repos/jaegertracing/jaeger/releases/latest | jq -r '.tag_name' | cut -c 2-) && \
    curl -L -o jaeger.tar.gz https://github.com/jaegertracing/jaeger/releases/download/v${JAEGER_VERSION}/jaeger-${JAEGER_VERSION}-linux-amd64.tar.gz && \
    tar -xzf jaeger.tar.gz && \
    rm jaeger.tar.gz && \
    mv jaeger-${JAEGER_VERSION}-linux-amd64/* /jaeger/ && \
    rmdir jaeger-${JAEGER_VERSION}-linux-amd64


FROM quay.io/konveyor/analyzer-lsp AS analyzer-builder

COPY --from=jaeger-builder /jaeger/* /usr/local/bin/

WORKDIR /analyzer-lsp

COPY rule-example.yaml /analyzer-lsp/rule-example.yaml
COPY examples /analyzer-lsp/examples
COPY open-source-libs.txt /analyzer-lsp/open-source-libs.txt

EXPOSE 5775/udp 6831/udp 6832/udp 5778 16686 14268 9411

ENTRYPOINT ["sh", "-c", "jaeger-all-in-one & sleep 10 && konveyor-analyzer --enable-jaeger=true --jaeger-endpoint=http://localhost:14268/api/traces && curl -o traces.json http://localhost:16686/api/traces?service=analyzer-lsp"]