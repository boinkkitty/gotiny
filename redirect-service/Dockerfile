FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY proto/ /build/proto/
COPY pkg/ /build/pkg/
COPY redirect-service/ /build/redirect-service/
WORKDIR /build/redirect-service
RUN go build -o /app ./cmd

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /app /app
ENTRYPOINT ["/app"]
