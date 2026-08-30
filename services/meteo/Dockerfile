# Multi-stage build for high-performance Meteo Weather API and Ingestion Worker
FROM golang:1.22-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

# Build API server and worker daemon
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /build/bin/meteo-api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /build/bin/meteo-worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /build/bin/zarr-dump ./cmd/tool

# Production runtime image
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/bin/meteo-api /app/meteo-api
COPY --from=builder /build/bin/meteo-worker /app/meteo-worker
COPY --from=builder /build/bin/zarr-dump /app/zarr-dump

# Default data store directory
VOLUME ["/data/store"]

ENV PORT=4081
ENV DATA_DIR=/data/store

EXPOSE 4081

CMD ["/app/meteo-api"]
