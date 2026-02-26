FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cron-weather ./cmd/cron-weather

FROM alpine:3.20
RUN apk add --no-cache tzdata
WORKDIR /app
COPY --from=build /out/cron-weather /app/cron-weather

ENV ENV=prod
CMD ["/app/cron-weather"]