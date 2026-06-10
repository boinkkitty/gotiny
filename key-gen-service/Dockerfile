FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY proto/ /build/proto/
COPY pkg/ /build/pkg/
COPY key-gen-service/ /build/key-gen-service/
WORKDIR /build/key-gen-service
RUN go build -o /app ./cmd

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
