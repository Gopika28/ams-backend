# Multi-stage Docker build for AMS Go Backend
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build production binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ams-backend .

# Minimal runtime stage
FROM alpine:latest  

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/ams-backend .
COPY --from=builder /app/schema.sql .
COPY --from=builder /app/seed.sql .

EXPOSE 8080

CMD ["./ams-backend"]
