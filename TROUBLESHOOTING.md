# Bot System Troubleshooting

## Bot Won't Connect

### Symptom
Bot-hoster starts but bots don't connect to backend

### Diagnosis
```bash
docker logs virusgame-bot-hoster
# Look for: "Failed to connect to..."
```

### Solution
- Check BACKEND_URL is correct
- Verify backend is running and accessible
- Check firewall rules
- Test connectivity: `telnet backend-host 8080`

## Bot Joins But Doesn't Move

### Symptom
Bot appears in lobby, game starts, but bot doesn't make moves

### Diagnosis
```bash
# Check bot logs
docker logs virusgame-bot-hoster
# Look for: "Calculating move..." messages

# Check backend logs
docker logs virusgame-backend
# Look for: "move" messages from bot
```

### Solution
- Verify AI engine initialized (Task 4)
- Check bot receives turn_change messages
- Verify bot's yourPlayer number is correct
- Check for errors in move calculation

## Bot Pool Exhausted

### Symptom
"Add Bot" clicked but no bot joins

### Diagnosis
```bash
# Check bot-hoster stats
docker logs virusgame-bot-hoster | grep "Pool stats"
# Look for: "Idle=0"
```

### Solution
- Deploy more bot-hoster instances
- Increase BOT_POOL_SIZE
- Wait for current games to finish
