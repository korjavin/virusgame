# Multi-stage Dockerfile for Virus Game with Go backend

# Stage 1: Build the Go backend
FROM golang:1.21-alpine AS go-builder

WORKDIR /build

# Copy go mod files
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy backend source
COPY backend/*.go ./

# Build the Go binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o virusgame-server .

# Stage 2: Build the WASM AI module
FROM golang:1.21-alpine AS wasm-builder

WORKDIR /wasm

# Copy WASM source
COPY wasm/go.mod wasm/ai.go ./

# Build WASM module
RUN GOOS=js GOARCH=wasm go build -o ai.wasm ai.go

# Stage 3: Create final image
FROM alpine:latest

# Add ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy the Go binary from builder
COPY --from=go-builder /build/virusgame-server .

# Copy WASM module and create wasm directory
RUN mkdir -p wasm
COPY --from=wasm-builder /wasm/ai.wasm ./wasm/

# Copy all frontend files (HTML, CSS, JS)
COPY index.html style.css favicon.jpg ./
COPY script.js ai.js ai-wasm.js multiplayer.js tutorial.js translations.js wasm_exec.js ./
COPY DOCS.md README.md ./

# Add build argument for commit SHA
ARG COMMIT_SHA=unknown

# Replace the placeholder in the HTML file with the commit SHA
RUN sed -i "s/__COMMIT_SHA__/${COMMIT_SHA}/g" /app/index.html

# Expose the port
EXPOSE 8080

# Run the Go server
CMD ["./virusgame-server"]
