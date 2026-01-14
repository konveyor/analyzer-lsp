FROM registry.redhat.io/ubi9/go-toolset:1.23 AS builder
COPY --chown=1001:0 . /workspace

# FIXME: golang dependency provider, generic-external-provider and java-external-provider need to be cleaned from this, they build on their own in release-0.5
WORKDIR /workspace

ENV GOEXPERIMENT strictfipsruntime
RUN CGO_ENABLED=1 go build -tags strictfipsruntime -o konveyor-analyzer ./cmd/analyzer/main.go
RUN CGO_ENABLED=1 go build -tags strictfipsruntime -o konveyor-analyzer-dep ./cmd/dep/main.go
RUN cd external-providers/golang-dependency-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && CGO_ENABLED=1 go build -tags strictfipsruntime -o golang-dependency-provider main.go
RUN cd external-providers/generic-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && CGO_ENABLED=1 go build -tags strictfipsruntime -o generic-external-provider main.go
RUN cd external-providers/java-external-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && CGO_ENABLED=1 go build -tags strictfipsruntime -o java-external-provider main.go

# FIXME: Runtime mta-jdtls-server-base (To be removed in release-0.5 pending)
FROM brew.registry.redhat.io/rh-osbs/mta-mta-jdtls-server-base-rhel9:8.0.0
RUN dnf -y install python3-devel gcc-c++

RUN mkdir /analyzer-lsp

COPY --from=builder /workspace/konveyor-analyzer /usr/local/bin/konveyor-analyzer
COPY --from=builder /workspace/konveyor-analyzer-dep /usr/local/bin/konveyor-analyzer-dep
COPY --from=builder /workspace/external-providers/generic-external-provider/generic-external-provider /usr/local/bin/generic-external-provider
COPY --from=builder /workspace/external-providers/golang-dependency-provider/golang-dependency-provider /usr/local/bin/golang-dependency-provider
COPY --from=builder /workspace/external-providers/java-external-provider/java-external-provider /usr/local/bin/java-external-provider
COPY --from=builder /workspace/provider_container_settings.json /analyzer-lsp/provider_settings.json
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
