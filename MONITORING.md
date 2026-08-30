# Bot System Monitoring

## Metrics to Track

### Backend
- Active WebSocket connections
- Bot users vs human users
- Games with bots vs games without
- Average game duration with bots

### Bot-Hoster
- Pool size (total bots)
- Idle bots count
- Active bots count (in games)
- Average move calculation time
- Reconnection frequency

## Logging

### Important Log Messages

**Backend:**
```
[INFO] User connected: AI-BotName (bot detected)
[INFO] Broadcasted bot_wanted for lobby X
[INFO] Bot AI-BotName joined lobby X
```

**Bot-Hoster:**
```
[INFO] Bot registered as AI-BotName
[INFO] Received bot_wanted signal for lobby X
[INFO] Joined lobby X
[INFO] Game started as player N
[INFO] Calculating move...
[INFO] Sent move: (X, Y)
```

### Error Patterns

**Connection Issues:**
```
[ERROR] Failed to connect to backend
[ERROR] Read error: EOF
[WARN] Reconnection attempt N/10
```

**Game Issues:**
```
[ERROR] No valid moves available
[ERROR] Move calculation timeout
[ERROR] Invalid move rejected
```

## Alerts

Set up alerts for:
1. All bots disconnected (pool size = 0)
2. High move calculation time (>5 seconds average)
3. High reconnection frequency (>10/minute)
4. Bot-hoster down (no heartbeat)
