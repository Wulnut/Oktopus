FROM golang:1.23 AS builder
WORKDIR /app
COPY ../ .
RUN CGO_ENABLED=0 GOOS=linux go build -o ws-adapter cmd/ws-adapter/main.go

FROM alpine:3.21
COPY --from=builder /app/ws-adapter /
ENTRYPOINT ["/ws-adapter"]