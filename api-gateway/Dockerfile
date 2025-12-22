FROM golang:1.25.3-alpine AS builder

WORKDIR /app

RUN go mod init api-gateway || true

COPY . .

RUN go mod tidy

RUN go build -o api-gateway cmd/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/api-gateway .
EXPOSE 8080
CMD ["./api-gateway"]
