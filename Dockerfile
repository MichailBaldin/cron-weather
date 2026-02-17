FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache gcc musl-dev sqlite-dev

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=1
RUN go build -ldflags="-s -w" -o cron-weather ./cmd/cron-weather.go

FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata sqlite-libs

WORKDIR /root
COPY --from=builder /app/cron-weather ./cron-weather

RUN mkdir -p /data
ENV DB_PATH=/data/subscriptions.db

CMD ["./cron-weather"]