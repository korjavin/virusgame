# Task 2: Shared Docker Image - Support Bot-Hoster Command

## Context

Currently, we have a backend service with its own Dockerfile. We want to **reuse the same Docker image** for both the backend server AND the bot-hoster service. This is achieved by creating a single image with multiple possible commands.

**Goal**: One Dockerfile, two services:
- `docker run virusgame backend` → Runs game server (current behavior)
- `docker run virusgame bot-hoster` → Runs bot pool manager

## Benefits

- Single codebase, single image
- Shared dependencies and AI code
- Easier maintenance and deployment
- Smaller total image size

## Related Files

- `backend/Dockerfile` - Current backend Dockerfile
- `backend/docker-compose.yml` - Current backend compose file
- `backend/main.go` - Backend entry point
- Will create: `backend/cmd/bot-hoster/main.go` - Bot-hoster entry point

## Architecture

```
virusgame/
├── backend/
│   ├── Dockerfile              # Builds BOTH binaries
│   ├── docker-compose.yml      # Backend service
│   ├── main.go                 # Backend entry point
│   ├── hub.go                  # Shared game logic
│   ├── bot.go                  # Shared AI engine
│   ├── types.go                # Shared types
│   └── cmd/
│       └── bot-hoster/
│           ├── main.go         # Bot-hoster entry point
│           ├── manager.go      # Bot pool manager
│           └── bot_client.go   # Bot WebSocket client
│
├── bot-hoster-compose.yml      # NEW: Bot-hoster deployment
└── .env.bot-hoster            # NEW: Bot-hoster config
```

## Goal

Create a shared Docker image that can run both backend and bot-hoster services, with separate docker-compose files for deployment.

## Acceptance Criteria

1. ✅ Single Dockerfile builds both `backend` and `bot-hoster` binaries
2. ✅ `docker-compose.yml` in `backend/` runs backend service (default behavior unchanged)
3. ✅ `bot-hoster-compose.yml` in root runs bot-hoster service
4. ✅ Bot-hoster connects to backend via configurable DNS name
5. ✅ Both services share same AI code (`bot.go`)
6. ✅ Services can run on different hosts
7. ✅ Easy to deploy with Portainer

## Implementation Steps

### Step 1: Update Dockerfile to Build Both Services

**File**: `backend/Dockerfile`

**Current** (example):
```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o backend .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/backend .
CMD ["./backend"]
```

**New** (multi-binary):
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy dependency files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o backend .
RUN CGO_ENABLED=0 GOOS=linux go build -o bot-hoster ./cmd/bot-hoster

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy both binaries
COPY --from=builder /app/backend .
COPY --from=builder /app/bot-hoster .

# Default command (can be overridden)
CMD ["./backend"]
```

### Step 2: Create Bot-Hoster Entry Point

**File**: `backend/cmd/bot-hoster/main.go`

```go
package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    log.Println("Starting bot-hoster service...")

    config := LoadConfig()
    manager := NewBotManager(config)

    // Start bot pool
    if err := manager.Start(); err != nil {
        log.Fatalf("Failed to start bot manager: %v", err)
    }

    log.Printf("Bot-hoster started with %d bots connected to %s",
        config.PoolSize, config.BackendURL)

    // Wait for interrupt signal
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
    <-sigChan

    log.Println("Shutting down bot-hoster...")
    manager.Stop()
    log.Println("Bot-hoster stopped")
}
```

**Note**: The actual implementation of `LoadConfig()`, `NewBotManager()`, etc. will be in Task 3. For now, we're just creating the structure.

### Step 3: Create Bot-Hoster Config

**File**: `backend/cmd/bot-hoster/config.go`

```go
package main

import (
    "os"
    "strconv"
)

type Config struct {
    BackendURL  string
    PoolSize    int
}

func LoadConfig() *Config {
    backendURL := getEnv("BACKEND_URL", "ws://localhost:8080/ws")
    poolSize, _ := strconv.Atoi(getEnv("BOT_POOL_SIZE", "10"))

    return &Config{
        BackendURL: backendURL,
        PoolSize:   poolSize,
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
```

### Step 4: Create Bot-Hoster Docker Compose

**File**: `bot-hoster-compose.yml` (in project root)

```yaml
version: '3.8'

services:
  bot-hoster:
    build:
      context: ./backend
      dockerfile: Dockerfile
    image: virusgame:latest
    container_name: virusgame-bot-hoster
    command: ["./bot-hoster"]  # Override default command
    restart: unless-stopped
    environment:
      - BACKEND_URL=${BACKEND_URL:-ws://virusgame-backend:8080/ws}
      - BOT_POOL_SIZE=${BOT_POOL_SIZE:-10}
    networks:
      - virusgame-network
    deploy:
      resources:
        limits:
          cpus: '2.0'
          memory: 2G
        reservations:
          cpus: '1.0'
          memory: 1G

networks:
  virusgame-network:
    external: true
```

### Step 5: Create Bot-Hoster Environment File

**File**: `.env.bot-hoster` (in project root)

```env
# Bot-Hoster Configuration

# Backend WebSocket URL (DNS name or IP)
# For local testing: ws://localhost:8080/ws
# For Docker network: ws://virusgame-backend:8080/ws
# For external server: ws://your-server.com:8080/ws
BACKEND_URL=ws://virusgame-backend:8080/ws

# Number of bots in the pool
# Each bot maintains a WebSocket connection
# Recommended: 10-20 per instance
BOT_POOL_SIZE=10
```

### Step 6: Update Backend docker-compose (if needed)

**File**: `backend/docker-compose.yml`

Ensure backend has a named network that bot-hoster can join:

```yaml
version: '3.8'

services:
  backend:
    build: .
    container_name: virusgame-backend
    restart: unless-stopped
    ports:
      - "8080:8080"
    networks:
      - virusgame-network

networks:
  virusgame-network:
    name: virusgame-network
    driver: bridge
```

### Step 7: Create Deployment Instructions

**File**: `DEPLOYMENT.md` (update or create)

Add section:

```markdown
## Deploying Bot-Hoster

### Prerequisites

1. Backend must be running
2. Backend must be accessible at the configured URL

### Option 1: Same Host as Backend

# Start backend
cd backend
docker-compose up -d

# Start bot-hoster (connects via Docker network)
cd ..
docker-compose -f bot-hoster-compose.yml --env-file .env.bot-hoster up -d

### Option 2: Different Host (e.g., Portainer)

1. Copy `bot-hoster-compose.yml` to target host
2. Configure `.env.bot-hoster`:
   ```env
   BACKEND_URL=ws://your-game-server.com:8080/ws
   BOT_POOL_SIZE=20
   ```
3. Deploy via Portainer or:
   ```bash
   docker-compose -f bot-hoster-compose.yml --env-file .env.bot-hoster up -d
   ```

### Scaling

Run multiple bot-hoster instances:

# Instance 1
docker-compose -f bot-hoster-compose.yml -p bot-hoster-1 up -d

# Instance 2
docker-compose -f bot-hoster-compose.yml -p bot-hoster-2 up -d

# Now you have 20 bots total (10 per instance)

### Monitoring

# Check bot-hoster logs
docker logs -f virusgame-bot-hoster

# Should see:
# Starting bot-hoster service...
# Bot 1 connected: AI-BraveOctopus42
# Bot 2 connected: AI-CleverWolf19
# ...
# Bot-hoster started with 10 bots connected to ws://backend:8080/ws
```

## Testing Steps

### Test 1: Build Docker Image

```bash
cd backend
docker build -t virusgame:test .

# Verify both binaries exist
docker run --rm virusgame:test ls -la
# Should show: backend, bot-hoster
```

### Test 2: Run Backend (Verify Default)

```bash
cd backend
docker-compose up

# Should start backend service as before
# Verify: http://localhost:8080 works
```

### Test 3: Run Bot-Hoster Locally

```bash
# Start backend first
cd backend
docker-compose up -d

# Start bot-hoster
cd ..
docker-compose -f bot-hoster-compose.yml --env-file .env.bot-hoster up

# Should see:
# Starting bot-hoster service...
# (Will error for now since bot manager not implemented yet - that's OK!)
```

### Test 4: Verify Network Connectivity

```bash
# Backend and bot-hoster should be on same network
docker network inspect virusgame-network

# Should show both containers
```

### Test 5: External Host Configuration

Edit `.env.bot-hoster`:
```env
BACKEND_URL=ws://192.168.1.100:8080/ws
BOT_POOL_SIZE=5
```

Run bot-hoster:
```bash
docker-compose -f bot-hoster-compose.yml --env-file .env.bot-hoster up
```

Verify it tries to connect to external IP (will fail if backend not there).

## File Checklist

After this task, you should have:

- [x] `backend/Dockerfile` - Updated to build both binaries
- [x] `backend/cmd/bot-hoster/main.go` - Bot-hoster entry point
- [x] `backend/cmd/bot-hoster/config.go` - Configuration loading
- [x] `bot-hoster-compose.yml` - Bot-hoster deployment config
- [x] `.env.bot-hoster` - Bot-hoster environment variables
- [x] `DEPLOYMENT.md` - Deployment instructions (updated)

## Stub Files (For Task 3)

Task 3 will implement these, but create stubs for now:

**File**: `backend/cmd/bot-hoster/manager.go`

```go
package main

import "log"

type BotManager struct {
    config *Config
    // TODO: Add bot pool
}

func NewBotManager(config *Config) *BotManager {
    return &BotManager{
        config: config,
    }
}

func (m *BotManager) Start() error {
    log.Println("BotManager.Start() - TODO: Implement in Task 3")
    return nil
}

func (m *BotManager) Stop() {
    log.Println("BotManager.Stop() - TODO: Implement in Task 3")
}
```

This allows the bot-hoster binary to compile and run (even if it doesn't do anything yet).

## Dependencies

**Blocked by**: None - can start immediately

**Blocks**:
- Task 3 (Bot-hoster implementation will use this structure)

## Estimated Time

**2-3 hours**

- 1 hour: Dockerfile and build setup
- 1 hour: Docker compose files and configuration
- 30 min: Testing and documentation
- 30 min: Stub files and verification

## Success Validation

```bash
# 1. Image builds successfully
cd backend
docker build -t virusgame:latest .

# 2. Backend runs (default command)
docker run --rm -p 8080:8080 virusgame:latest
# Visit http://localhost:8080 - should work

# 3. Bot-hoster runs (override command)
docker run --rm virusgame:latest ./bot-hoster
# Should print: "Starting bot-hoster service..."

# 4. Docker compose works
docker-compose -f bot-hoster-compose.yml --env-file .env.bot-hoster up
# Should start bot-hoster container
```

## Notes

- The same image is used for both services - this is intentional and efficient
- Bot-hoster will share the AI code from `backend/bot.go` (already in the image)
- For Portainer deployment, use `bot-hoster-compose.yml` as a stack
- Environment variables can be overridden in Portainer UI

## Common Issues

**Issue**: Bot-hoster can't connect to backend
**Solution**: Check `BACKEND_URL` matches backend's actual address:
- Same host + Docker: `ws://virusgame-backend:8080/ws`
- Different host: `ws://backend-ip-or-dns:8080/ws`

**Issue**: Port conflicts
**Solution**: Bot-hoster doesn't expose ports (it's a client), so no conflicts should occur

**Issue**: Network not found
**Solution**: Run backend first to create the network, or create it manually:
```bash
docker network create virusgame-network
```

## Related Documentation

- `BOT_HOSTER_PLAN_V2.md` - Architecture overview
- Docker documentation: https://docs.docker.com/compose/
- Portainer stacks: https://docs.portainer.io/user/docker/stacks
