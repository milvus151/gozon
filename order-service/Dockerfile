FROM golang:1.25.3-alpine AS builder

WORKDIR /app

RUN go mod init order-service || true

RUN go get github.com/lib/pq
RUN go get github.com/rabbitmq/amqp091-go

COPY . .

RUN go mod tidy

RUN go build -o order-service cmd/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/order-service .
EXPOSE 8080
CMD ["./order-service"]
