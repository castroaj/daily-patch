# Dockerfile — container image for the api service
#
# Multi-stage build: compiles the Go binary in a builder stage, then copies
# it into a minimal alpine image. Exposes port 8080.

FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN go build -o bin/api .

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/bin/api .
EXPOSE 8080
CMD ["./api"]
