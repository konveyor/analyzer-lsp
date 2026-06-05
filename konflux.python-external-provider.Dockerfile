FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
COPY --chown=1001:0 . /workspace

WORKDIR /workspace/external-providers/python-external-provider
ENV GOEXPERIMENT strictfipsruntime
RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && \
    CGO_ENABLED=1 go build -mod=readonly -tags strictfipsruntime -a -o python-external-provider main.go

FROM registry.redhat.io/ubi9/ubi:latest

RUN dnf install -y python3 python3-pip openssl && \
    dnf reinstall -y tzdata && \
    dnf clean all && \
    rm -rf /var/cache/dnf
RUN python3 -m pip install --no-cache-dir 'python-lsp-server>=1.8.2'

WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001

COPY --from=builder /workspace/external-providers/python-external-provider/python-external-provider /usr/local/bin/python-external-provider
COPY --from=builder /workspace/LICENSE /licenses/

ENV HOME /addon
ENTRYPOINT ["/usr/local/bin/python-external-provider"]

LABEL \
        description="Migration Toolkit for Applications - Python External Provider" \
        io.k8s.description="Migration Toolkit for Applications - Python External Provider" \
        io.k8s.display-name="MTA - Python External Provider" \
        io.openshift.maintainer.project="MTA" \
        io.openshift.tags="migration,modernization,mta,tackle,konveyor" \
        summary="Migration Toolkit for Applications - Python External Provider"
