# TODO: fix version mismatch in final container
FROM golang:1.21 as go-builder

COPY / /analyzer-lsp

WORKDIR /dotnet-provider

COPY external-providers/dotnet-external-provider/Makefile .
COPY external-providers/dotnet-external-provider/go.mod .
COPY external-providers/dotnet-external-provider/go.sum .
COPY external-providers/dotnet-external-provider/main.go .
COPY external-providers/dotnet-external-provider/pkg/ ./pkg

RUN go mod edit --replace=github.com/konveyor/analyzer-lsp=/analyzer-lsp && go mod tidy

RUN make build
RUN ADD_PLAT=yes OS=windows make build

# TODO(djzager): resurrect if we need omnisharp-roslyn
# FROM registry.access.redhat.com/ubi8/dotnet-70 AS builder
# USER root
# RUN microdnf -y install dnf
# RUN dnf -y install 'dnf-command(config-manager)'
# RUN dnf config-manager --set-enabled ubi-8-codeready-builder-rpms
# RUN dnf -y install https://dl.fedoraproject.org/pub/epel/epel-release-latest-8.noarch.rpm
# RUN dnf -y install mono-complete
# USER default
# RUN curl -s -L -O https://github.com/OmniSharp/omnisharp-roslyn/archive/refs/tags/v1.39.6.tar.gz \
#   && tar -xf v1.39.6.tar.gz
# RUN cd omnisharp-roslyn-1.39.6 && ./build.sh --target Build --use-global-dotnet-sdk

FROM registry.access.redhat.com/ubi8/dotnet-80
USER root
# This is because SDKs are not universally available across architectures
# RUN if [[ "$(uname -m)" = "x86_64" || "$(uname -m)" = "amd64" ]]; then \
#       microdnf -y install dotnet-sdk-2.1 dotnet-sdk-2.1.5xx dotnet-sdk-3.0 \
#                           dotnet-sdk-3.1 dotnet-sdk-5.0 dotnet-sdk-6.0 \
#                           dotnet-sdk-7.0; \
#     elif [[ "$(uname -m)" = "aarch64" || "$(uname -m)" = "s390x" ]]; then \
#       microdnf -y install dotnet-sdk-6.0 dotnet-sdk-7.0; \
#     elif [[ "$(uname -m)" = "ppc64le" ]]; then \
#       microdnf -y install dotnet-sdk-7.0; \
#     fi \
RUN microdnf -y install dotnet-sdk-7.0 \
    && microdnf clean all \
    && rm -rf /var/cache/yum
RUN dotnet tool install --global csharp-ls --version 0.11.0
ENV PATH="$PATH:/opt/app-root/.dotnet/tools:/home/go/bin"
USER default
EXPOSE 3456

#COPY --from=builder /opt/app-root/src/omnisharp-roslyn-1.39.6/bin/Release/OmniSharp.Stdio.Driver/net6.0/ /opt/app-root/omnisharp
COPY --from=go-builder /dotnet-provider/bin/ /usr/bin/
ENTRYPOINT ["dotnet-external-provider", "-port", "3456"]
