# Deployment Guide

This guide covers deploying the Virus Game with multiplayer support using Docker.

## Prerequisites

- Docker installed
- Docker Compose installed (if using Traefik)
- Access to push images to `ghcr.io/korjavin/virusgame` (or change to your registry)

## What Changed for Multiplayer

### New Architecture
- **Before**: Nginx serving static files only
- **After**: Go backend serving static files + WebSocket connections

### Updated Files
- ✅ [Dockerfile](Dockerfile) - Multi-stage build (Go + Alpine)
- ✅ [docker-compose.yml](docker-compose.yml) - WebSocket support labels
- ✅ [backend/main.go](backend/main.go) - Docker-aware file serving

## Building the Docker Image

### Local Build

```bash
# Build with commit SHA
docker build --build-arg COMMIT_SHA=$(git rev-parse --short HEAD) -t virusgame:latest .

# Test locally
docker run -p 8080:8080 virusgame:latest
```

Then open `http://localhost:8080` in your browser.

### Production Build

```bash
# Build and tag for registry
docker build --build-arg COMMIT_SHA=$(git rev-parse --short HEAD) \
  -t ghcr.io/korjavin/virusgame:latest \
  -t ghcr.io/korjavin/virusgame:$(git rev-parse --short HEAD) .

# Push to registry
docker push ghcr.io/korjavin/virusgame:latest
docker push ghcr.io/korjavin/virusgame:$(git rev-parse --short HEAD)
```

## Deployment Options

### Option 1: Simple Docker Run

```bash
docker run -d \
  --name virusgame \
  -p 8080:8080 \
  --restart unless-stopped \
  ghcr.io/korjavin/virusgame:latest
```

### Option 2: Docker Compose (No Traefik)

Create a simple `docker-compose.yml`:

```yaml
version: "3.8"

services:
  virusgame:
    image: ghcr.io/korjavin/virusgame:latest
    container_name: virusgame
    ports:
      - "8080:8080"
    restart: unless-stopped
```

Deploy:
```bash
docker-compose up -d
```

### Option 3: Docker Compose with Traefik (Current Setup)

The existing [docker-compose.yml](docker-compose.yml) is configured for Traefik reverse proxy with:
- HTTPS/TLS via Let's Encrypt
- WebSocket support
- Custom hostname via `${HOSTNAME}` environment variable

**Environment Variables Required:**
```bash
export HOSTNAME=yourdomain.com
export NETWORK_NAME=traefik_network  # or your Traefik network name
```

**Deploy:**
```bash
docker-compose up -d
```

## WebSocket Configuration

### Traefik Labels Explained

```yaml
# Enable Traefik for this service
- "traefik.enable=true"

# Route HTTP requests based on hostname
- "traefik.http.routers.virusgame.rule=Host(`${HOSTNAME}`)"

# Use HTTPS endpoint
- "traefik.http.routers.virusgame.entrypoints=websecure"

# TLS/SSL certificate resolver
- "traefik.http.routers.virusgame.tls.certresolver=myresolver"

# Specify the backend port
- "traefik.http.services.virusgame.loadbalancer.server.port=8080"

# WebSocket support - Connection upgrade headers
- "traefik.http.middlewares.virusgame-ws.headers.customrequestheaders.Connection=upgrade"
- "traefik.http.middlewares.virusgame-ws.headers.customrequestheaders.Upgrade=websocket"

# Apply WebSocket middleware to router
- "traefik.http.routers.virusgame.middlewares=virusgame-ws"
```

### Without Traefik (Direct Access)

If not using Traefik, the Go server handles WebSocket upgrades natively. No additional configuration needed - just expose port 8080.

## Testing the Deployment

### 1. Check Container Status

```bash
docker ps | grep virusgame
docker logs virusgame
```

You should see:
```
Server starting on :8080
Serving static files from: /app
```

### 2. Test HTTP Endpoint

```bash
curl -I http://your-domain.com
# or for local
curl -I http://localhost:8080
```

Should return HTTP 200.

### 3. Test WebSocket Connection

Open browser console at your deployed URL:
```javascript
const ws = new WebSocket('wss://your-domain.com/ws');
ws.onopen = () => console.log('Connected!');
ws.onmessage = (e) => console.log('Message:', e.data);
```

You should see a welcome message with your assigned username.

### 4. Test Multiplayer

1. Open the deployed URL in two browser windows
2. Each should show a different random username
3. Challenge from one window
4. Accept in the other
5. Play the game - moves should sync in real-time

## Monitoring

### View Logs

```bash
# Follow logs
docker logs -f virusgame

# Last 100 lines
docker logs --tail 100 virusgame
```

### Useful Log Messages

```
Server starting on :8080
Serving static files from: /app
User connected: BraveOctopus42 (uuid)
Challenge created: User1 -> User2
Game started: User1 vs User2 (Game ID: uuid)
User disconnected: BraveOctopus42 (uuid)
```

## Updating the Deployment

```bash
# Pull latest image
docker pull ghcr.io/korjavin/virusgame:latest

# Restart container
docker-compose down
docker-compose up -d

# Or with docker run
docker stop virusgame
docker rm virusgame
docker run -d --name virusgame -p 8080:8080 ghcr.io/korjavin/virusgame:latest
```

## Scaling Considerations

### Current Setup
- Single container
- In-memory game state
- WebSocket connections pinned to container

### For High Traffic

If you need to scale beyond one container:

1. **Use Redis for state**: Store game sessions and user lists in Redis
2. **Redis Pub/Sub**: Broadcast moves between containers
3. **Sticky sessions**: Configure load balancer for WebSocket affinity
4. **Database**: Optional persistent storage for game history

### Example Redis Setup

```yaml
services:
  redis:
    image: redis:alpine
    container_name: virusgame-redis

  virusgame:
    image: ghcr.io/korjavin/virusgame:latest
    environment:
      - REDIS_URL=redis:6379
    depends_on:
      - redis
```

(Requires code changes to use Redis)

## Firewall Configuration

Ensure these ports are open:
- **80**: HTTP (if using Traefik)
- **443**: HTTPS (if using Traefik)
- **8080**: Direct access (if not using Traefik)

## Security Considerations

### Production Checklist

- ✅ Run behind HTTPS/TLS (Traefik handles this)
- ✅ WebSocket connections are encrypted (WSS over HTTPS)
- ⚠️ Add rate limiting for WebSocket connections
- ⚠️ Implement authentication for production use
- ⚠️ Add CORS configuration if needed
- ⚠️ Configure proper logging and monitoring

### Environment-Specific Config

You can use environment variables for production settings:

```go
// Example: Add to main.go
port := os.Getenv("PORT")
if port == "" {
    port = "8080"
}
```

## Troubleshooting

### Container Won't Start

```bash
# Check logs
docker logs virusgame

# Common issues:
# - Port 8080 already in use
# - Missing files in /app
# - Go binary not executable
```

### WebSocket Connection Fails

1. Check Traefik labels are correct
2. Verify WebSocket middleware is applied
3. Check browser console for CORS errors
4. Test WebSocket endpoint directly: `wscat -c wss://your-domain.com/ws`

### Users Can't See Each Other

1. Verify both users are on the same server instance
2. Check WebSocket connections are established
3. Look for errors in server logs
4. Check browser console for disconnection messages

## Rollback

```bash
# List available image tags
docker images | grep virusgame

# Deploy specific version
docker-compose down
docker pull ghcr.io/korjavin/virusgame:<commit-sha>
# Update docker-compose.yml with specific tag
docker-compose up -d
```

## CI/CD Integration

Your GitHub Actions workflow should:

1. Build Docker image with commit SHA
2. Push to registry (ghcr.io)
3. Update deployment (pull + restart)

Example workflow:
```yaml
- name: Build and push
  run: |
    docker build --build-arg COMMIT_SHA=${{ github.sha }} \
      -t ghcr.io/korjavin/virusgame:latest \
      -t ghcr.io/korjavin/virusgame:${{ github.sha }} .
    docker push ghcr.io/korjavin/virusgame:latest
    docker push ghcr.io/korjavin/virusgame:${{ github.sha }}
```

## Support

For issues or questions:
- Check server logs: `docker logs virusgame`
- Review [MULTIPLAYER.md](MULTIPLAYER.md) for architecture
- See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues
