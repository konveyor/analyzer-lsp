FROM registry.redhat.io/ubi10/go-toolset:1.23 AS go-builder
COPY --chown=1001:0 . /workspace

WORKDIR /workspace/external-providers/generic-external-provider
ENV GOEXPERIMENT strictfipsruntime
RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && CGO_ENABLED=1 go build -tags strictfipsruntime -o generic-external-provider main.go

WORKDIR /workspace/hack/build/tools/gopls
RUN CGO_ENABLED=1 go build -tags strictfipsruntime -buildvcs=false

FROM brew.registry.redhat.io/rh-osbs/mta-mta-golang-dependency-provider-rhel9:8.0.0 as go-dep-provider

FROM registry.redhat.io/ubi10:latest
RUN dnf -y module enable nodejs:18
RUN dnf -y install openssl gcc-c++ python-devel python3-devel nodejs && dnf -y clean all

# Python LSP server
COPY --from=go-builder /workspace/hack/build/python-lsp-server.tgz python-lsp-server.tgz
RUN tar xzvf python-lsp-server.tgz
RUN pip install -r python-lsp-server/requirements.txt --no-index --find-links python-lsp-server
RUN rm -r python-lsp-server.tgz python-lsp-server

# Typescript LSP server
ENV NODEJS_VERSION=18
COPY --from=go-builder /workspace/hack/build/typescript.tgz typescript.tgz
COPY --from=go-builder /workspace/hack/build/typescript-language-server.tgz typescript-language-server.tgz
RUN npm install -g typescript-language-server.tgz typescript.tgz
RUN typescript-language-server --version
RUN rm -r typescript.tgz typescript-language-server.tgz

COPY --from=go-builder /workspace/external-providers/generic-external-provider/generic-external-provider /usr/local/bin/generic-external-provider
COPY --from=go-builder /workspace/hack/build/tools/gopls/gopls /usr/local/bin/gopls
COPY --from=go-builder /workspace/external-providers/generic-external-provider/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY --from=go-builder /workspace/LICENSE /licenses/
COPY --from=go-dep-provider /usr/local/bin/golang-dependency-provider /usr/local/bin/golang-dependency-provider

WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon

USER 1001
ENV HOME /addon

ENTRYPOINT ["/usr/local/bin/generic-external-provider"]

LABEL \
        description="Migration Toolkit for Applications - Generic External Provider" \
        io.k8s.description="Migration Toolkit for Applications - Generic External Provider" \
        io.k8s.display-name="MTA - Generic External Provider" \
        io.openshift.maintainer.project="MTA" \
        io.openshift.tags="migration,modernization,mta,tackle,konveyor" \
        summary="Migration Toolkit for Applications - Generic External Provider"
