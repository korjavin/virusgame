# Multi-stage Dockerfile for Virus Game with Go backend

# Stage 1: Build the Go backend
FROM golang:1.24-alpine AS go-builder

WORKDIR /build

# Copy go mod files
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy backend source
COPY backend/ .

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o virusgame-server .

# Build the bot-hoster binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o bot-hoster ./cmd/bot-hoster

# Stage 2: Create final image
FROM alpine:latest

# Add ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the Go binary from builder
COPY --from=go-builder /build/virusgame-server .
COPY --from=go-builder /build/bot-hoster .

# Copy all frontend files (HTML, CSS, JS)
COPY index.html style.css favicon.jpg ./
COPY script.js ai.js multiplayer.js lobby.js tutorial.js translations.js ./
COPY DOCS.md README.md ./

# Add build argument for commit SHA
ARG COMMIT_SHA=unknown

# Replace the placeholder in the HTML file with the commit SHA
RUN sed -i "s/__COMMIT_SHA__/${COMMIT_SHA}/g" /app/index.html

# Expose the port
EXPOSE 8080

# Run the Go server
CMD ["./virusgame-server"]
