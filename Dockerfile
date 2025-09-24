# syntax=docker/dockerfile:1

FROM golang:1.24 AS builder
WORKDIR /src

ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN mkdir -p /out \
    && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X github.com/arrgo/arrgo/internal/buildinfo.Version=${VERSION} -X github.com/arrgo/arrgo/internal/buildinfo.Commit=${COMMIT} -X github.com/arrgo/arrgo/internal/buildinfo.BuildTime=${BUILD_DATE}" -o /out/arrgo-master ./cmd/arrgo-master \
    && GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X github.com/arrgo/arrgo/internal/buildinfo.Version=${VERSION} -X github.com/arrgo/arrgo/internal/buildinfo.Commit=${COMMIT} -X github.com/arrgo/arrgo/internal/buildinfo.BuildTime=${BUILD_DATE}" -o /out/arrgo-worker ./cmd/arrgo-worker

FROM alpine:3.22
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

LABEL org.opencontainers.image.title="arrgo" \
      org.opencontainers.image.description="arrgo master/worker services" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${COMMIT}" \
      org.opencontainers.image.created="${BUILD_DATE}"

RUN apk add --no-cache ca-certificates tzdata \
    && adduser -S -u 1000 appuser

COPY --from=builder /out/arrgo-master /usr/local/bin/arrgo-master
COPY --from=builder /out/arrgo-worker /usr/local/bin/arrgo-worker
COPY deploy/docker/entrypoint.sh /usr/local/bin/arrgo-entrypoint

RUN chmod +x /usr/local/bin/arrgo-entrypoint

USER appuser

ENTRYPOINT ["/usr/local/bin/arrgo-entrypoint"]
