# Multiplayer Quickstart Guide

## What's New

Your Virus Game now supports real-time multiplayer! Players can:
- ✅ Get automatically assigned random names (e.g., "BraveOctopus42")
- ✅ See all online players in a live list
- ✅ Challenge other players to games
- ✅ Accept or decline challenges
- ✅ Play in real-time with synchronized moves
- ✅ Request rematches after games end

## Quick Start (3 Steps)

### 1. Start the Server

```bash
./start-server.sh
```

Or manually:
```bash
cd backend
go run .
```

### 2. Open Your Browser

Navigate to: `http://localhost:8080`

### 3. Test Multiplayer

Open the same URL in **multiple browser windows** (or tabs, or different browsers) to simulate multiple players.

## How It Works

### When You Connect
1. Browser automatically connects to WebSocket server
2. Server assigns you a random username (shown in top-left)
3. Connection status shows "Connected" (green)

### Challenging Players
1. Look at the **Online Players** list on the left sidebar
2. Available players show a "Challenge" button
3. Players currently in games show "(In Game)" status
4. Click "Challenge" next to any available player
5. Confirm the challenge dialog

### Receiving Challenges
1. Challenge notifications appear in the **top-right corner**
2. You have 30 seconds to Accept or Decline
3. Click "Accept" to start the game
4. Click "Decline" to reject

### Playing the Game
1. Game starts when challenge is accepted
2. You're assigned either Player 1 (X, blue) or Player 2 (O, red)
3. Status bar shows whose turn it is
4. You can only make moves on your turn
5. Moves are synchronized in real-time with your opponent

### After Game Ends
1. Winner is announced
2. "Request Rematch" button appears
3. Click to send rematch request to your opponent
4. If they accept, a new game starts immediately

## Architecture Overview

### Backend (Go)
- **Location**: `/backend`
- **Port**: 8080
- **WebSocket endpoint**: `/ws`
- **Files**:
  - `main.go` - Server entry point
  - `hub.go` - Game logic and message routing
  - `client.go` - WebSocket connection handling
  - `types.go` - Data structures
  - `names.go` - Random name generator

### Frontend (JavaScript)
- **Location**: `/multiplayer.js`
- **Features**:
  - WebSocket client with auto-reconnect
  - Real-time UI updates
  - Challenge management
  - Game state synchronization

### Message Protocol
All communication uses JSON over WebSockets. See [MULTIPLAYER.md](MULTIPLAYER.md) for detailed protocol documentation.

## Testing

### Local Testing
1. Open `http://localhost:8080` in **two browser windows**
2. You'll see two different usernames assigned
3. Each window shows the other in the online players list
4. Challenge yourself and play!

### Network Testing
1. Find your local IP: `ifconfig | grep inet`
2. Start the server
3. On another device on the same network, open: `http://YOUR_IP:8080`

## Troubleshooting

### "Disconnected" Status
- Check that the Go server is running
- Look for "Server starting on :8080" in terminal
- Verify no other service is using port 8080

### Can't See Other Players
- Ensure you're on the same server instance
- Check browser console for WebSocket errors (F12)
- Verify both clients show "Connected" status

### Moves Not Syncing
- Check browser console for errors
- Verify it's your turn (status bar shows current player)
- Ensure you're not trying to move in local mode

### Server Won't Start
```bash
# Check if port is in use
lsof -i :8080

# Kill any process using the port
kill -9 <PID>

# Try starting again
cd backend && go run .
```

## File Structure

```
virusgame/
├── backend/                 # Go backend server
│   ├── main.go             # Server entry point
│   ├── hub.go              # Central game hub
│   ├── client.go           # WebSocket client
│   ├── types.go            # Data structures
│   ├── names.go            # Name generator
│   ├── go.mod              # Go dependencies
│   └── README.md           # Backend docs
├── multiplayer.js          # Frontend WebSocket client
├── script.js               # Game logic (updated for MP)
├── index.html              # UI with sidebar
├── style.css               # Styles with MP components
├── MULTIPLAYER.md          # Detailed architecture
├── MULTIPLAYER_QUICKSTART.md  # This file
└── start-server.sh         # Quick start script
```

## Features in Detail

### Random Name Generation
Names follow the pattern: `[Adjective][Animal][Number]`
- 32 adjectives (Brave, Clever, Wild, etc.)
- 32 animals (Octopus, Tiger, Phoenix, etc.)
- Random number 0-99
- Examples: BraveOctopus42, CleverTiger88, WildPhoenix15

### Online Users List
- Shows all connected players except yourself
- Real-time updates when players join/leave
- Grays out players currently in games
- Shows challenge buttons for available players

### Challenge System
- Send challenges with confirmation dialog
- Receive challenges as notifications (top-right)
- 30-second auto-decline timeout
- Can't challenge players already in games

### Game Sessions
- Server maintains authoritative game state
- Validates all moves server-side
- Synchronized board state between players
- Handles disconnections gracefully

### Rematch System
- Appears after game ends
- Sends rematch request to opponent
- Creates new game when accepted
- Returns to lobby if declined

## Next Steps

### Deployment
To deploy to production:
1. Build the Go binary: `cd backend && go build -o server`
2. Deploy to your hosting platform
3. Update WebSocket URL in frontend to use production domain
4. Configure HTTPS and WSS for security

### Enhancements
Potential improvements:
- Persistent user accounts
- Game history and statistics
- Spectator mode
- Chat system
- Ranked matchmaking
- Tournament system

## Support

For detailed architecture and message protocol, see:
- [MULTIPLAYER.md](MULTIPLAYER.md) - Full architecture documentation
- [backend/README.md](backend/README.md) - Backend server documentation
- [README.md](README.md) - Main game documentation

## License

Same as the main project. See [LICENSE](LICENSE).
