# Task 6: Testing, Documentation, and Deployment

## Context

This is the final task that ensures everything works together, is well-documented, and ready for production deployment.

## Prerequisites

- Tasks 1-5 completed
- Bot system fully functional
- Bot creation guide published

## Goal

Thoroughly test the entire bot system, document deployment procedures, and prepare for production use.

## Acceptance Criteria

1. ✅ All integration tests pass
2. ✅ Load testing shows system can handle expected traffic
3. ✅ Deployment documentation is complete
4. ✅ Monitoring and logging work correctly
5. ✅ Rollback procedures documented
6. ✅ Performance benchmarks recorded

## Test Categories

### 1. Integration Tests

#### Test 1.1: Single Bot End-to-End

**Scenario**: One bot, one human, full game

```bash
# Setup
1. Start backend
2. Start bot-hoster (1 bot)
3. Human creates lobby
4. Human clicks "Add Bot"
5. Start game
6. Play full game

# Expected Results
✓ Bot joins lobby within 1 second
✓ Bot appears in lobby UI
✓ Game starts successfully
✓ Bot makes moves on its turns (3 per turn)
✓ Human can make moves normally
✓ Game completes with winner
✓ Bot returns to idle state
✓ Bot can join another game

# Pass Criteria
- All steps complete without errors
- Game plays normally
- Bot responds within 2 seconds per move
```

#### Test 1.2: Multiple Bots in Same Game

**Scenario**: 1 human + 3 bots, 4-player game

```bash
# Setup
1. Start backend
2. Start bot-hoster (10 bots)
3. Create lobby (4 slots)
4. Click "Add Bot" 3 times
5. Human joins
6. Start game

# Expected Results
✓ 3 different bots join
✓ All bots show as ready
✓ Game starts with 4 players
✓ Each bot plays on correct turn
✓ No interference between bots
✓ Game progresses normally
✓ All bots return to pool after game

# Pass Criteria
- All 3 bots join successfully
- Each bot makes independent moves
- Game completes normally
```

#### Test 1.3: Bot Pool Exhaustion

**Scenario**: More lobbies than available bots

```bash
# Setup
1. Start bot-hoster (2 bots)
2. Create lobby A, add bot → Bot 1 joins
3. Create lobby B, add bot → Bot 2 joins
4. Create lobby C, add bot → ???

# Expected Results
✓ Lobby A gets Bot 1
✓ Lobby B gets Bot 2
✓ Lobby C signal sent but no bot available
✓ No crashes or errors
✓ User can try again when bot frees up

# Pass Criteria
- System handles gracefully
- No errors in logs
- Can retry later successfully
```

#### Test 1.4: Concurrent Games

**Scenario**: 5 games running simultaneously with bots

```bash
# Setup
1. Start backend
2. Start bot-hoster (10 bots)
3. Create 5 lobbies, each with 1 human + 1 bot
4. Start all games
5. Play all games concurrently

# Expected Results
✓ All 5 bots join their respective lobbies
✓ All 5 games start
✓ Each bot plays only in its assigned game
✓ No move cross-contamination
✓ All games complete successfully
✓ All bots return to pool

# Pass Criteria
- 5 concurrent games work
- No data corruption
- Each bot independent
```

#### Test 1.5: Bot Reconnection

**Scenario**: Network interruption during game

```bash
# Setup
1. Start backend
2. Start bot-hoster (1 bot)
3. Bot joins game, starts playing
4. Kill backend (Ctrl+C)
5. Wait 5 seconds
6. Restart backend

# Expected Results
✓ Bot detects disconnection
✓ Bot attempts reconnection
✓ Bot reconnects successfully
✓ Bot registers again (new userId)
✓ Bot ready for new games

# Pass Criteria
- Bot recovers from disconnection
- No manual intervention needed
- Bot operational after reconnect
```

### 2. Load Tests

#### Test 2.1: 50 Concurrent Bots

**Scenario**: High load simulation

```bash
# Setup
1. Start backend
2. Start 5 bot-hoster instances (10 bots each)
3. Create 25 lobbies (2 players each: 1 human + 1 bot)
4. Start all 25 games
5. Monitor for 10 minutes

# Metrics to Collect
- Backend CPU usage
- Backend memory usage
- Bot-hoster CPU usage (per instance)
- Bot-hoster memory usage (per instance)
- Average move calculation time
- Backend response time
- WebSocket message throughput

# Pass Criteria
- Backend CPU < 50%
- Backend memory < 2GB
- Bot-hoster CPU < 80% per instance
- Bot-hoster memory < 1.5GB per instance
- Move calculation < 3 seconds (95th percentile)
- No crashes or errors
- All games complete successfully
```

#### Test 2.2: Bot Pool Scaling

**Scenario**: Scale from 10 to 100 bots

```bash
# Setup
1. Start backend
2. Start bot-hoster-1 (10 bots)
3. Verify 10 bots available
4. Start bot-hoster-2 (10 bots)
5. Verify 20 bots available
6. Continue scaling to 10 instances (100 bots total)

# Expected Results
✓ Each new instance registers independently
✓ Backend tracks all instances
✓ Bot pool size increases linearly
✓ No conflicts between instances
✓ All bots can join games

# Pass Criteria
- All 100 bots connect successfully
- No duplicate usernames/IDs
- Load distributed across instances
```

#### Test 2.3: Rapid Bot Addition

**Scenario**: Stress test bot_wanted signal

```bash
# Setup
1. Start backend
2. Start bot-hoster (20 bots)
3. Create lobby
4. Click "Add Bot" rapidly 20 times in 10 seconds

# Expected Results
✓ All 20 signals broadcast
✓ 4 bots join (lobby max)
✓ Remaining signals ignored (lobby full)
✓ No race conditions
✓ No duplicate bots in lobby

# Pass Criteria
- System handles rapid requests
- Lobby never overfills
- No errors or crashes
```

### 3. Performance Benchmarks

Run these tests and record results:

#### Benchmark 1: Bot Join Latency

```
Measure: Time from "Add Bot" click to bot appearing in lobby

Target: < 500ms (95th percentile)
Acceptable: < 1000ms (95th percentile)
```

#### Benchmark 2: Move Calculation Time

```
Measure: Time to calculate move (depth 5, 20x20 board)

Target: < 1000ms (average)
Acceptable: < 2000ms (95th percentile)
```

#### Benchmark 3: Message Throughput

```
Measure: Messages/second backend can handle

Target: > 1000 msg/sec
Acceptable: > 500 msg/sec
```

#### Benchmark 4: Concurrent Games

```
Measure: Number of concurrent bot games backend can handle

Target: > 50 games
Acceptable: > 30 games
```

### 4. Edge Case Tests

#### Test 4.1: Invalid Moves

```bash
# Scenario: Bot tries to make invalid move

# Setup
1. Modify bot to send invalid move (e.g., out of bounds)
2. Start game

# Expected
✓ Backend rejects invalid move
✓ Backend sends error message
✓ Game continues (bot can retry)
✓ No crash or state corruption
```

#### Test 4.2: Move Timeout

```bash
# Scenario: Bot takes too long to move

# Setup
1. Modify bot AI to sleep 2 minutes before moving
2. Start game

# Expected
✓ Backend has move timeout (120 seconds)
✓ Backend auto-resigns slow player
✓ Game continues with remaining players
✓ Bot is kicked from game
```

#### Test 4.3: Malformed Messages

```bash
# Scenario: Bot sends invalid JSON

# Setup
1. Modify bot to send malformed message
2. Trigger bot to send message

# Expected
✓ Backend handles gracefully
✓ Backend logs error
✓ Connection remains open
✓ Bot can send valid message after
```

#### Test 4.4: Rapid Reconnect

```bash
# Scenario: Bot connects/disconnects rapidly

# Setup
1. Bot connects
2. Bot disconnects immediately
3. Repeat 100 times in 10 seconds

# Expected
✓ Backend handles gracefully
✓ No memory leaks
✓ Connection limits respected
✓ No DOS attack vector
```

## Documentation Updates

### Update 1: DEPLOYMENT.md

Add sections:

```markdown
## Bot-Hoster Deployment

### Production Checklist

- [ ] Backend deployed and accessible
- [ ] Bot-hoster docker-compose.yml configured
- [ ] BACKEND_URL points to production server
- [ ] BOT_POOL_SIZE set appropriately
- [ ] Resource limits configured
- [ ] Monitoring enabled
- [ ] Logs aggregated

### Deployment Steps

1. Prepare environment
2. Build Docker image
3. Deploy via Portainer
4. Verify connectivity
5. Monitor logs
6. Test bot joining

### Scaling Strategy

- 1 bot-hoster per 20 concurrent games
- Deploy on separate hosts for isolation
- Use load balancer if >5 instances

### Monitoring

Watch these metrics:
- Bot pool size (idle vs active)
- Move calculation times
- Reconnection frequency
- Error rates
```

### Update 2: TROUBLESHOOTING.md

Create troubleshooting guide:

```markdown
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
```

### Update 3: MONITORING.md

Create monitoring guide:

```markdown
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
```

## Deployment Verification

After deploying to production, verify:

### Checklist

- [ ] Backend accessible from bot-hoster
- [ ] Bot-hoster starts without errors
- [ ] Bots register successfully (check backend logs)
- [ ] Bot pool size matches BOT_POOL_SIZE
- [ ] Can create lobby and add bot
- [ ] Bot joins lobby within 1 second
- [ ] Bot plays full game successfully
- [ ] Multiple bots can join different games
- [ ] Monitoring dashboards show metrics
- [ ] Logs are being collected
- [ ] Alerts configured and tested

### Smoke Test Script

```bash
#!/bin/bash
# smoke_test.sh - Verify bot system is working

set -e

echo "=== Bot System Smoke Test ==="

echo "1. Check backend health..."
curl -f http://backend:8080/ > /dev/null && echo "✓ Backend is up"

echo "2. Check bot-hoster is running..."
docker ps | grep bot-hoster && echo "✓ Bot-hoster is running"

echo "3. Check bots are connected..."
BOTS=$(docker logs virusgame-bot-hoster 2>&1 | grep "Bot registered" | wc -l)
echo "✓ $BOTS bots connected"

if [ "$BOTS" -eq 0 ]; then
    echo "✗ No bots connected! Check logs."
    exit 1
fi

echo "4. Check for recent errors..."
ERRORS=$(docker logs --since 5m virusgame-bot-hoster 2>&1 | grep ERROR | wc -l)
if [ "$ERRORS" -gt 0 ]; then
    echo "✗ Found $ERRORS errors in last 5 minutes"
    exit 1
fi
echo "✓ No recent errors"

echo ""
echo "=== All checks passed! ==="
```

## Performance Tuning

### Backend Tuning

If backend is slow with many bots:

```yaml
# docker-compose.yml
deploy:
  resources:
    limits:
      cpus: '4.0'      # Increase if needed
      memory: 4G       # Increase if needed
```

### Bot-Hoster Tuning

If move calculations are slow:

```env
# .env.bot-hoster
BOT_POOL_SIZE=10          # Reduce if CPU-bound
# or deploy more instances
```

If running out of memory:

```yaml
# bot-hoster-compose.yml
deploy:
  resources:
    limits:
      memory: 4G       # Increase
```

## Rollback Plan

If bot system causes issues:

### Option 1: Disable Bot-Hoster

```bash
docker-compose -f bot-hoster-compose.yml down
```

Game continues to work, just no bots available.

### Option 2: Revert Backend Changes

```bash
cd backend
git revert <commit-hash>  # Revert Task 1 changes
docker-compose up -d --build
```

### Option 3: Use Old Bot System

If old bot system still exists, switch back:

```bash
# Re-enable old handleAddBot logic
# Remove bot_wanted broadcast
git checkout <old-commit>
docker-compose up -d --build
```

## Dependencies

**Blocked by**: Tasks 1-5

**Blocks**: None (Final task)

## Estimated Time

**6-8 hours**

- 3 hours: Integration testing
- 2 hours: Load testing
- 2 hours: Documentation
- 1 hour: Performance benchmarks

## Success Criteria

### Must Have
- ✅ All integration tests pass
- ✅ Load test with 50 concurrent bots succeeds
- ✅ Documentation complete
- ✅ Deployment verified on staging
- ✅ Rollback procedure tested

### Nice to Have
- ⏳ 100+ concurrent bots tested
- ⏳ Monitoring dashboards set up
- ⏳ Automated smoke tests
- ⏳ Performance tuning guide

## Deliverables

- [ ] Test results document
- [ ] Performance benchmarks
- [ ] Updated DEPLOYMENT.md
- [ ] TROUBLESHOOTING.md
- [ ] MONITORING.md
- [ ] Smoke test script
- [ ] Production deployment checklist

## Final Validation

Before marking project complete:

```bash
# 1. All tests pass
./run_all_tests.sh

# 2. Load test
./load_test.sh --bots=50 --duration=10m

# 3. Smoke test
./smoke_test.sh

# 4. Documentation review
# - All markdown files render correctly
# - No broken links
# - Code examples work

# 5. Production ready
# - Deployed to staging
# - Verified with real users
# - Performance acceptable
# - Monitoring working
```

## Notes

- This task ensures production readiness
- Don't skip load testing - it reveals issues
- Document everything - future you will thank you
- Performance benchmarks are baseline for future optimization

## Related Documentation

- `BOT_HOSTER_PLAN_V2.md` - Architecture
- All task files (todo_task1.md through todo_task5.md)
- Backend and bot-hoster source code
