# Build the manager binary
FROM golang:1.21 as builder

WORKDIR /java-provider

COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download

COPY main.go main.go
COPY pkg/ pkg/

RUN go build -a -o java-external-provider main.go

FROM quay.io/konveyor/jdtls-server-base

COPY --from=builder /java-provider/java-external-provider /usr/local/bin/java-external-provider

EXPOSE 14651

ENTRYPOINT ["java-external-provider", "--port", "14651"]
