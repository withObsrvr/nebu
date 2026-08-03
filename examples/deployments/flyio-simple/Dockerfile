# Build the processors used by this deployment from their canonical modules in
# nebu-processor-registry. The official nebu image supplies the CLI only.
FROM golang:1.25.4-alpine AS processor-builder

ENV CGO_ENABLED=0 GOBIN=/processors
RUN go install github.com/withObsrvr/nebu-processor-registry/processors/contract-events/cmd/contract-events@latest && \
    go install github.com/withObsrvr/nebu-processor-registry/processors/postgres-sink/cmd/postgres-sink@latest

FROM withobsrvr/nebu:latest AS nebu-cli

FROM alpine:3.19
RUN apk add --no-cache bash ca-certificates postgresql-client

COPY --from=nebu-cli /usr/local/bin/nebu /usr/local/bin/nebu
COPY --from=processor-builder /processors/contract-events /usr/local/bin/contract-events
COPY --from=processor-builder /processors/postgres-sink /usr/local/bin/postgres-sink
COPY start.sh /start.sh
COPY 001_schema.sql /001_schema.sql

RUN chmod +x /start.sh /usr/local/bin/* && \
    addgroup -g 1000 nebu && \
    adduser -D -u 1000 -G nebu nebu && \
    mkdir -p /data && \
    chown -R nebu:nebu /data

USER nebu
WORKDIR /data
ENTRYPOINT ["/start.sh"]
