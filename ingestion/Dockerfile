# Dockerfile — container image for the ingestion service
#
# Multi-stage build: compiles the Go binary in a builder stage, then copies
# it into a minimal alpine image. Runs as a one-shot container.

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o bin/ingestion .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/bin/ingestion .
CMD ["./ingestion"]
