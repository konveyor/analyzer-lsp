FROM registry.redhat.io/ubi10/go-toolset:1.23 AS builder
COPY --chown=1001:0 . /workspace

WORKDIR /workspace/external-providers/java-external-provider
ENV GOEXPERIMENT strictfipsruntime
RUN go mod edit -replace=github.com/konveyor/analyzer-lsp=../../ && CGO_ENABLED=1 go build -tags strictfipsruntime -a -o java-external-provider main.go

FROM brew.registry.redhat.io/rh-osbs/mta-mta-jdtls-server-base-rhel9:8.0.0

WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001

COPY --from=builder /workspace/external-providers/java-external-provider/java-external-provider /usr/local/bin/java-external-provider
COPY --from=builder /workspace/LICENSE /licenses/

ENV HOME /addon
EXPOSE 14651
ENTRYPOINT ["java-external-provider", "--port", "14651"]

LABEL \
        description="Migration Toolkit for Applications - Java External Provider" \
        io.k8s.description="Migration Toolkit for Applications - Java External Provider" \
        io.k8s.display-name="MTA - Java External Provider" \
        io.openshift.maintainer.project="MTA" \
        io.openshift.tags="migration,modernization,mta,tackle,konveyor" \
        summary="Migration Toolkit for Applications - Java External Provider"
