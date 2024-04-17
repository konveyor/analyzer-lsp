FROM golang:1.20 as go-builder

WORKDIR /go-dependency-provider

COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

COPY main.go main.go
RUN go build -o go-dependency-provider main.go

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

COPY --from=go-builder /go-dependency-provider/go-dependency-provider /usr/local/bin/go-dependency-provider

ENTRYPOINT [ "/usr/local/bin/go-dependency-provider" ]