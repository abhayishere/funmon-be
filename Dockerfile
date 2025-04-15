# Stage 1: Builder
FROM golang:1.21 as builder

WORKDIR /app

# Copy go.mod and go.sum first for caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the code
COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o main .

# Stage 2: Final slim image
FROM debian:bullseye-slim

# Install CA certificates (Debian package manager = apt)
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binary and env
COPY --from=builder /app/main .
COPY .env .env

EXPOSE 8080

CMD ["./main"]
