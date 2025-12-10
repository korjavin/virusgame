# Bot Template

Quick-start template for building your own game bot in Go.

## Quick Start

```bash
# 1. Clone or copy this template
cp -r bot-template my-bot
cd my-bot

# 2. Install dependencies
go mod init my-bot
go get github.com/gorilla/websocket

# 3. Configure
echo "BACKEND_URL=ws://your-server.com/ws" > .env

# 4. Run
go run .
```

## Files

- `main.go` - Entry point
- `bot.go` - Bot client logic
- `ai.go` - AI strategy (customize this!)
- `protocol.go` - Message definitions
- `Dockerfile` - Docker build

## Customize Your Bot

Edit `ai.go` to implement your strategy:

```go
func (b *Bot) calculateBestMove() (int, int) {
    // Your AI logic here!
    // Return best (row, col)
}
```

## Deploy

```bash
docker build -t my-bot .
docker run -e BACKEND_URL=ws://server.com/ws my-bot
```

## Learn More

See `/BOT_DEVELOPMENT_GUIDE.md` for full documentation.
