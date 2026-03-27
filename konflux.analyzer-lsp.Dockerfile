FROM registry.redhat.io/ubi10/go-toolset:1.23 AS builder
COPY --chown=1001:0 . /workspace

WORKDIR /workspace

ENV GOEXPERIMENT strictfipsruntime
RUN CGO_ENABLED=1 go build -tags strictfipsruntime -o konveyor-analyzer ./cmd/analyzer/main.go
RUN CGO_ENABLED=1 go build -tags strictfipsruntime -o konveyor-analyzer-dep ./cmd/dep/main.go

FROM registry.redhat.io/ubi10/ubi:latest
RUN dnf -y install python3-devel gcc-c++

RUN mkdir /analyzer-lsp

COPY --from=builder /workspace/konveyor-analyzer /usr/local/bin/konveyor-analyzer
COPY --from=builder /workspace/konveyor-analyzer-dep /usr/local/bin/konveyor-analyzer-dep
#COPY --from=builder /workspace/provider_container_settings.json /analyzer-lsp/provider_settings.json
COPY --from=builder /workspace/LICENSE /licenses/

WORKDIR /analyzer-lsp
RUN chgrp -R 0 /analyzer-lsp && chmod -R g=u /analyzer-lsp

ENTRYPOINT ["sh", "-c", "konveyor-analyzer"]

LABEL \
        description="Migration Toolkit for Applications - Analyzer LSP" \
        io.k8s.description="Migration Toolkit for Applications - Analyzer LSP" \
        io.k8s.display-name="MTA - Analyzer LSP" \
        io.openshift.maintainer.project="MTA" \
        io.openshift.tags="migration,modernization,mta,tackle,konveyor" \
        summary="Migration Toolkit for Applications - Analyzer LSP"
