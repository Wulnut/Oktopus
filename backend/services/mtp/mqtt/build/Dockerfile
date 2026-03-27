FROM golang:1.23 as builder
WORKDIR /app
COPY ../ .
RUN CGO_ENABLED=0 GOOS=linux go build -o mqtt cmd/mqtt/main.go

FROM alpine:3.21
COPY --from=builder /app/mqtt /
ENTRYPOINT ["/mqtt"]
