# --- Build stage ---
FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o flip-chat-server ./cmd/server

# --- Runtime stage ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=builder /app/flip-chat-server .
COPY --from=builder /app/db/migrations ./db/migrations

RUN chown -R appuser:appgroup /app
USER appuser

EXPOSE 8080
CMD ["./flip-chat-server"]