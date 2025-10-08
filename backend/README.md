# Virus Game - Backend Server

This is the Go backend server for the Virus Game multiplayer mode.

## Features

- WebSocket-based real-time communication
- Automatic random username generation (e.g., "BraveOctopus42")
- Online user list with availability status
- Game challenge system (send, accept, decline)
- Real-time game session management
- Move synchronization between players
- Rematch functionality
- Automatic reconnection handling

## Architecture

### Components

- **main.go**: Entry point, HTTP and WebSocket server setup
- **hub.go**: Central message hub, handles all game logic and user management
- **client.go**: WebSocket client connection management
- **types.go**: Data structures and message types
- **names.go**: Random username generation

### Message Flow

1. **Connection**: Client connects → Server assigns random name → Broadcasts user list
2. **Challenge**: User A challenges User B → Server notifies B → B accepts/declines
3. **Game Start**: Both players receive game session details and initial board state
4. **Gameplay**: Players send moves → Server validates → Broadcasts to opponent
5. **Game End**: Winner determined → Rematch option available

## Running the Server

```bash
# Install dependencies
go mod download

# Run the server
go run .

# Or build and run
go build -o virusgame-server
./virusgame-server
```

The server runs on port 8080 by default and serves:
- WebSocket endpoint: `ws://localhost:8080/ws`
- Static files: `http://localhost:8080/` (serves parent directory)

## Configuration

To change the port, modify [main.go](main.go:20):
```go
err := http.ListenAndServe(":8080", nil)
```

## WebSocket Protocol

See [MULTIPLAYER.md](../MULTIPLAYER.md) for detailed message protocol documentation.

## Development

### Adding New Features

1. Define message types in [types.go](types.go)
2. Add message handlers in [hub.go](hub.go)
3. Update frontend client in [../multiplayer.js](../multiplayer.js)

### Testing

To test locally, open multiple browser windows to `http://localhost:8080` and challenge yourself!

## Dependencies

- [gorilla/websocket](https://github.com/gorilla/websocket) - WebSocket implementation
- [google/uuid](https://github.com/google/uuid) - UUID generation for IDs
