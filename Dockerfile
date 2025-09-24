FROM golang:1.23 AS builder
WORKDIR /src

ENV CGO_ENABLED=0
ARG TARGETOS=linux
ARG TARGETARCH=amd64

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN mkdir -p /out \
    && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/arrgo-master ./cmd/arrgo-master \
    && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/arrgo-worker ./cmd/arrgo-worker

FROM alpine:3.22
LABEL org.opencontainers.image.title="arrgo" \
      org.opencontainers.image.description="arrgo master/worker services"

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -S appuser

COPY --from=builder /out/arrgo-master /usr/local/bin/arrgo-master
COPY --from=builder /out/arrgo-worker /usr/local/bin/arrgo-worker
COPY deploy/docker/entrypoint.sh /usr/local/bin/arrgo-entrypoint

RUN chmod +x /usr/local/bin/arrgo-entrypoint

USER appuser

ENTRYPOINT ["/usr/local/bin/arrgo-entrypoint"]
