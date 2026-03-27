FROM golang:1.23 as builder
WORKDIR /app
COPY ../ .
RUN CGO_ENABLED=0 GOOS=linux go build -o http-bulk-collector cmd/http-bulk-collector/main.go

FROM alpine:3.21
COPY --from=builder /app/http-bulk-collector /
ENTRYPOINT ["/bulkdata"]