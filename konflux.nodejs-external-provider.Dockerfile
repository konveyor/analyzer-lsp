FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
COPY --chown=1001:0 . /workspace

WORKDIR /workspace/external-providers/nodejs-external-provider
ENV GOEXPERIMENT strictfipsruntime
RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && \
    CGO_ENABLED=1 go build -mod=readonly -tags strictfipsruntime -a -o nodejs-external-provider main.go

FROM registry.redhat.io/ubi9/ubi:latest

ENV NODEJS_VERSION=18
RUN echo -e "[nodejs]\nname=nodejs\nstream=${NODEJS_VERSION}\nprofiles=\nstate=enabled\n" > /etc/dnf/modules.d/nodejs.module
RUN dnf install -y nodejs npm openssl && \
    dnf reinstall -y tzdata && \
    dnf clean all && \
    rm -rf /var/cache/dnf
RUN npm install -g typescript-language-server typescript

WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001

COPY --from=builder /workspace/external-providers/nodejs-external-provider/nodejs-external-provider /usr/local/bin/nodejs-external-provider
COPY --from=builder /workspace/LICENSE /licenses/

ENV HOME /addon
ENTRYPOINT ["/usr/local/bin/nodejs-external-provider"]

LABEL \
        description="Migration Toolkit for Applications - Node.js External Provider" \
        io.k8s.description="Migration Toolkit for Applications - Node.js External Provider" \
        io.k8s.display-name="MTA - Node.js External Provider" \
        io.openshift.maintainer.project="MTA" \
        io.openshift.tags="migration,modernization,mta,tackle,konveyor" \
        summary="Migration Toolkit for Applications - Node.js External Provider"
