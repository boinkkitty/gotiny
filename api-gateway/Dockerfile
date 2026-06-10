FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY proto/ /build/proto/
COPY api-gateway/ /build/api-gateway/
WORKDIR /build/api-gateway
RUN go build -o /app ./cmd

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
