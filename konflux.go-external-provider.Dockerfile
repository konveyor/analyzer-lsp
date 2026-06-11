FROM registry.access.redhat.com/ubi9/go-toolset:1.25 AS builder
COPY --chown=1001:0 . /workspace

WORKDIR /workspace/external-providers/go-external-provider
ENV GOEXPERIMENT strictfipsruntime
RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && CGO_ENABLED=1 go build -tags strictfipsruntime -a -o go-external-provider main.go && \
    GOBIN=/workspace/external-providers/go-external-provider CGO_ENABLED=1 go install -mod=readonly -tags strictfipsruntime golang.org/x/tools/gopls@v0.20.0

FROM registry.redhat.io/ubi9/ubi:latest

RUN dnf install -y openssl && \
    dnf reinstall -y tzdata && \
    dnf clean all && \
    rm -rf /var/cache/dnf

WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001

COPY --from=builder /workspace/external-providers/go-external-provider/go-external-provider /usr/local/bin/go-external-provider
COPY --from=builder /workspace/external-providers/go-external-provider/gopls /usr/local/bin/gopls
COPY --from=builder /usr/bin/go /usr/local/bin/go
COPY --from=builder /usr/lib/golang /usr/lib/golang
COPY --from=builder /workspace/LICENSE /licenses/

ENV HOME=/addon \
    GOROOT=/usr/lib/golang \
    GOPATH=/addon/go \
    GOCACHE=/addon/.cache/go-build \
    GOMODCACHE=/addon/go/pkg/mod

ENTRYPOINT ["/usr/local/bin/go-external-provider"]

LABEL \
        description="Migration Toolkit for Applications - Go External Provider" \
        io.k8s.description="Migration Toolkit for Applications - Go External Provider" \
        io.k8s.display-name="MTA - Go External Provider" \
        io.openshift.maintainer.project="MTA" \
        io.openshift.tags="migration,modernization,mta,tackle,konveyor" \
        summary="Migration Toolkit for Applications - Go External Provider"
