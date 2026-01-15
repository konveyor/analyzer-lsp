ARG JAVA_BUNDLE_TAG=latest
FROM quay.io/konveyor/jdtls-server-base:${JAVA_BUNDLE_TAG} as base

# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.23 as builder

USER 0

copy / /analyzer-lsp

RUN mkdir java
COPY --chmod=0777 external-providers/java-external-provider/go.mod java/go.mod
COPY external-providers/java-external-provider/go.sum java/go.sum


COPY external-providers/java-external-provider/main.go java/main.go
COPY external-providers/java-external-provider/pkg/ java/pkg/

RUN mkdir -p /opt/app-root/src/go && \
    chgrp -R 0 /opt/app-root/src/go && \
    chmod -R g=u /opt/app-root/src/go && \
    chgrp -R 0 java && \
    chmod -R g=u java

USER 0

# Fix cache ownership if it was populated by previous builds
RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod \
    chown -R 1001:0 /opt/app-root/src/go/pkg/mod && \
    chmod -R g+w /opt/app-root/src/go/pkg/mod

USER 1001

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd java && go mod edit -replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN --mount=type=cache,id=gomod,uid=1001,gid=0,mode=0777,target=/opt/app-root/src/go/pkg/mod cd java && go build -a -o java-external-provider main.go

FROM base

COPY --from=builder /opt/app-root/src/java/java-external-provider /usr/local/bin/java-external-provider
RUN mkdir -p /root/.gradle
COPY external-providers/java-external-provider/gradle/build.gradle /usr/local/etc/task.gradle
COPY external-providers/java-external-provider/gradle/build-v9.gradle /usr/local/etc/task-v9.gradle

ENV HOME /addon
EXPOSE 14651
WORKDIR /addon
ENTRYPOINT ["java-external-provider", "--port", "14651"]
