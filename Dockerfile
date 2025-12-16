# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app
RUN apk add --no-cache ca-certificates

# Copy all source files including vendor directory
COPY . .

# Build the application using vendor
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -a -installsuffix cgo -ldflags="-w -s" -o octoslack .

# Runtime stage
FROM scratch

# Copy the binary from builder
COPY --from=builder /app/octoslack /octoslack

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Run the application
ENTRYPOINT ["/octoslack"]
