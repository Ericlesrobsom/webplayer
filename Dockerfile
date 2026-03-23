FROM golang:1.21-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download 2>/dev/null || true
COPY main.go .
RUN CGO_ENABLED=1 go build -o iptv-webplayer .

FROM alpine:latest
RUN apk add --no-cache ca-certificates sqlite-libs
WORKDIR /app
COPY --from=builder /app/iptv-webplayer .
COPY static/ ./static/
COPY .env.example ./.env.example
EXPOSE 80
CMD ["./iptv-webplayer"]
