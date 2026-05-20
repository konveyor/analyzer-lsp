# Node.js gRPC provider binary image with typescript-language-server runtime.
FROM registry.access.redhat.com/ubi9/go-toolset:1.25 as go-builder

USER 0

RUN mkdir nodejs-provider
COPY / /analyzer-lsp

COPY --chmod=0777 external-providers/nodejs-external-provider/go.mod nodejs-provider/go.mod
COPY external-providers/nodejs-external-provider/go.sum nodejs-provider/go.sum

COPY external-providers/nodejs-external-provider/main.go nodejs-provider/main.go
COPY external-providers/nodejs-external-provider/pkg/ nodejs-provider/pkg/

RUN mkdir -p /opt/app-root/src/go && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 nodejs-provider && \
    chmod -R g=u nodejs-provider

USER 0

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd nodejs-provider && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd nodejs-provider && go build -o nodejs-external-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

ENV NODEJS_VERSION=18
RUN echo -e "[nodejs]\nname=nodejs\nstream=${NODEJS_VERSION}\nprofiles=\nstate=enabled\n" > /etc/dnf/modules.d/nodejs.module
RUN microdnf install nodejs npm -y && \
    microdnf clean all && \
    rm -rf /var/cache/dnf
RUN npm install -g typescript-language-server typescript

COPY --from=go-builder /opt/app-root/src/nodejs-provider/nodejs-external-provider /usr/local/bin/nodejs-external-provider
WORKDIR /addon
RUN chgrp -R 0 /addon && chmod -R g=u /addon
USER 1001
ENV HOME /addon

ENTRYPOINT ["/usr/local/bin/nodejs-external-provider"]
