FROM golang:1.23 AS builder
WORKDIR /app
COPY ../ .
RUN CGO_ENABLED=0 GOOS=linux go build -o file-server main.go

FROM alpine:3.21
COPY --from=builder /app/file-server /
ENTRYPOINT ["/file-server"]