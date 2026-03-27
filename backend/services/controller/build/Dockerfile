FROM golang:1.23 as builder
WORKDIR /app
COPY ../ .
RUN CGO_ENABLED=0 GOOS=linux go build -o controller cmd/controller/main.go

FROM alpine:3.21
RUN adduser -D appuser
COPY --from=builder /app/controller /
USER appuser
ENTRYPOINT ["/controller"]