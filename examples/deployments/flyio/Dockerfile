# Multi-stage Dockerfile for nebu indexer deployment
# This example builds contract-events and postgres-sink processors

# ============================================================================
# Stage 1: Build processors
# ============================================================================
# Using golang:alpine (latest) to ensure compatibility with nebu's Go version requirements
# If you need a specific Go version, change to golang:1.23-alpine or similar
FROM golang:alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev

# Clone the processor registry, which owns the published processor sources.
ARG REGISTRY_VERSION=main
RUN git clone --depth 1 --branch ${REGISTRY_VERSION} https://github.com/withObsrvr/nebu-processor-registry.git

WORKDIR /build/nebu-processor-registry

# Each processor has its own go.mod, so build from its directory.

# contract-events: Origin processor that extracts contract invocation events
WORKDIR /build/nebu-processor-registry/processors/contract-events
RUN go build -o /processors/contract-events \
    -ldflags="-s -w" \
    ./cmd/contract-events

# postgres-sink: Sink processor that writes to PostgreSQL
WORKDIR /build/nebu-processor-registry/processors/postgres-sink
RUN go build -o /processors/postgres-sink \
    -ldflags="-s -w" \
    ./cmd/postgres-sink

# Build transform processors for pipeline filtering
# These are optional and can be configured via PIPELINE_FILTERS env var

# dedup: Deduplicate events in stream
WORKDIR /build/nebu-processor-registry/processors/dedup
RUN go build -o /processors/dedup \
    -ldflags="-s -w" \
    ./cmd/dedup

# amount-filter: Filter events by amount threshold
WORKDIR /build/nebu-processor-registry/processors/amount-filter
RUN go build -o /processors/amount-filter \
    -ldflags="-s -w" \
    ./cmd/amount-filter

# ============================================================================
# Stage 2: Runtime image
# ============================================================================
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache \
    bash \
    curl \
    ca-certificates \
    postgresql-client \
    jq

# Create non-root user for running indexer
RUN addgroup -g 1000 nebu && \
    adduser -D -u 1000 -G nebu nebu

# Copy processors from builder stage
COPY --from=builder /processors/* /usr/local/bin/
RUN chmod +x /usr/local/bin/*

# Copy startup scripts
COPY scripts/ /scripts/
RUN chmod +x /scripts/*.sh && chown -R nebu:nebu /scripts

# Copy database migrations
COPY migrations/ /migrations/
RUN chown -R nebu:nebu /migrations

# Create data directory for checkpoints/logs
RUN mkdir -p /data && chown -R nebu:nebu /data

# Switch to non-root user
USER nebu

WORKDIR /data

# Health check - verify indexer process is running
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD /scripts/health-check.sh || exit 1

# Default command runs the indexer
CMD ["/scripts/start-indexer.sh"]
