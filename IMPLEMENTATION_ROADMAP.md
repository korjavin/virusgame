# Bot System Implementation Roadmap

## Overview

This document provides an overview of the bot system implementation, broken down into 6 independent tasks.

## Quick Links

- [Architecture Plan V2](BOT_HOSTER_PLAN_V2.md) - Complete architecture documentation
- [Task Files](#task-breakdown) - Individual task descriptions

## Project Goals

1. **Offload bot AI computation** from main backend to separate service
2. **Enable independent developers** to create their own bots
3. **Scale horizontally** by deploying multiple bot-hoster instances
4. **Maintain simplicity** - bots are first-class players using same protocol as humans

## Key Concept

**Bots = Users with AI**

Bots connect to the same `/ws` endpoint as humans and use the same game protocol. The only difference:
- Bots listen for `bot_wanted` signals
- Bots calculate moves using AI (instead of waiting for human input)

## Architecture Diagram

```
Main Backend (/ws)
    │
    ├─── Human Player (Browser)
    ├─── Human Player (Browser)
    ├─── Bot "AI-BraveOctopus" (Bot-Hoster)
    ├─── Bot "AI-CleverWolf" (Bot-Hoster)
    └─── Bot "AI-CustomBot" (External Developer's Bot)
```

All clients use the **same WebSocket protocol**!

## Task Breakdown

### Task 1: Backend - Broadcast bot_wanted Signal
**Time**: 1-2 hours | **Complexity**: Low | **Blockers**: None

Modify `handleAddBot()` to broadcast a `bot_wanted` signal instead of directly creating a bot slot.

**Key Change**: ~5 lines in `backend/hub.go`

**Files**: `todo_task1.md`

---

### Task 2: Shared Docker Image - Support Bot-Hoster Command
**Time**: 2-3 hours | **Complexity**: Low | **Blockers**: None

Create a single Docker image that can run both backend and bot-hoster services.

**Key Deliverables**:
- Updated Dockerfile (builds both binaries)
- `bot-hoster-compose.yml` for deployment
- `.env.bot-hoster` configuration

**Files**: `todo_task2.md`

---

### Task 3: Bot-Hoster Service - Core Implementation
**Time**: 6-8 hours | **Complexity**: High | **Blockers**: Task 1, 2

Implement the bot pool manager and bot client that:
- Connects to backend via WebSocket
- Listens for bot_wanted signals
- Joins lobbies
- Tracks game state

**Key Deliverables**:
- `backend/cmd/bot-hoster/main.go`
- `backend/cmd/bot-hoster/manager.go`
- `backend/cmd/bot-hoster/bot_client.go`

**Files**: `todo_task3.md`

---

### Task 4: Bot AI Integration - Make Bots Play
**Time**: 4-6 hours | **Complexity**: High | **Blockers**: Task 3

Integrate the AI engine so bots can calculate and send moves.

**Key Deliverables**:
- `backend/cmd/bot-hoster/ai_engine.go` (copy and adapt from `backend/bot.go`)
- Bot move calculation logic
- Full game playing capability

**Files**: `todo_task4.md`

---

### Task 5: Bot Creation Guide for Independent Developers
**Time**: 4-5 hours | **Complexity**: Low | **Blockers**: Task 4

Create documentation and template for external developers to build bots.

**Key Deliverables**:
- `BOT_DEVELOPMENT_GUIDE.md`
- `bot-template/` - Go template project
- Python/JavaScript examples

**Files**: `todo_task5.md`

---

### Task 6: Testing, Documentation, and Deployment
**Time**: 6-8 hours | **Complexity**: Medium | **Blockers**: Tasks 1-5

Comprehensive testing, performance benchmarks, and deployment verification.

**Key Deliverables**:
- Integration tests
- Load tests (50+ concurrent bots)
- Deployment documentation
- Monitoring guides
- Troubleshooting guides

**Files**: `todo_task6.md`

---

## Total Estimated Time

**23-32 hours** (3-4 days of focused work)

## Parallel Work Opportunities

Some tasks can be done in parallel:

```
Task 1 ──┐
Task 2 ──┴─→ Task 3 ──→ Task 4 ──┐
                                  ├─→ Task 6
Task 5 ───────────────────────────┘
```

- **Tasks 1 & 2** can be done in parallel (different developers)
- **Task 5** can start after Task 4 (while Task 6 is in progress)

## Success Metrics

### Functionality
- ✅ Bots join lobbies automatically
- ✅ Bots play full games successfully
- ✅ Multiple bots can be in same game
- ✅ Bots can be deployed on separate hosts

### Performance
- ✅ Main backend CPU reduced by 80%+ (bots vs no bots)
- ✅ Bot move calculation < 2 seconds (95th percentile)
- ✅ System handles 50+ concurrent bot games
- ✅ Bot pool scales horizontally

### Ecosystem
- ✅ External developers can build bots
- ✅ Bot template works out of the box
- ✅ Documentation is comprehensive

## Implementation Order

### Phase 1: Core Functionality (Tasks 1-4)
Goal: Working bot system

1. Task 1 (Backend changes) - 2 hours
2. Task 2 (Docker setup) - 3 hours
3. Task 3 (Bot client) - 8 hours
4. Task 4 (AI integration) - 6 hours

**Total**: ~19 hours

**Milestone**: Bots can join lobbies and play games

### Phase 2: Ecosystem & Production (Tasks 5-6)
Goal: Production-ready and developer-friendly

5. Task 5 (Developer guide) - 5 hours
6. Task 6 (Testing & deployment) - 8 hours

**Total**: ~13 hours

**Milestone**: System is production-ready and documented

## Risk Mitigation

### Risk 1: AI Performance Issues
**Mitigation**: Task 4 includes performance testing, can tune search depth

### Risk 2: WebSocket Connection Issues
**Mitigation**: Task 3 includes reconnection logic

### Risk 3: Bot Pool Exhaustion
**Mitigation**: Horizontal scaling (Task 2), graceful handling (Task 3)

### Risk 4: Protocol Changes Break Bots
**Mitigation**: Bot protocol is same as human protocol (stable)

## Rollback Strategy

If issues occur:
1. **Disable bot-hoster**: `docker-compose down` (no impact on human players)
2. **Revert backend**: Restore old `handleAddBot()` logic
3. **Fallback**: Old bot system can coexist temporarily

## Getting Started

### For Implementing

1. Read [BOT_HOSTER_PLAN_V2.md](BOT_HOSTER_PLAN_V2.md) for architecture overview
2. Start with Task 1 ([todo_task1.md](todo_task1.md))
3. Follow tasks in order (1 → 2 → 3 → 4)
4. Task 5 and 6 can overlap with Task 4

### For Understanding

1. Read this document for high-level overview
2. Read [BOT_HOSTER_PLAN_V2.md](BOT_HOSTER_PLAN_V2.md) for architecture details
3. Review specific task files for implementation details

## File Structure After Completion

```
virusgame/
├── backend/
│   ├── Dockerfile                    # Modified: builds both binaries
│   ├── docker-compose.yml           # Unchanged: backend service
│   ├── hub.go                       # Modified: broadcasts bot_wanted
│   ├── bot.go                       # Unchanged: AI engine
│   ├── types.go                     # Modified: add RequestID field
│   └── cmd/
│       └── bot-hoster/              # NEW: Bot-hoster service
│           ├── main.go
│           ├── manager.go
│           ├── bot_client.go
│           └── ai_engine.go
│
├── bot-hoster-compose.yml           # NEW: Bot-hoster deployment
├── .env.bot-hoster                  # NEW: Bot-hoster config
│
├── bot-template/                    # NEW: Developer template
│   ├── main.go
│   ├── bot.go
│   ├── ai.go
│   ├── protocol.go
│   ├── Dockerfile
│   └── README.md
│
├── BOT_HOSTER_PLAN_V2.md           # Architecture documentation
├── BOT_DEVELOPMENT_GUIDE.md        # NEW: Developer guide
├── IMPLEMENTATION_ROADMAP.md       # This file
│
├── todo_task1.md                   # Task descriptions
├── todo_task2.md
├── todo_task3.md
├── todo_task4.md
├── todo_task5.md
└── todo_task6.md
```

## Communication Protocol

### For Developers

The bot protocol is the **same as the human protocol** documented in `MULTIPLAYER.md`, with one addition:

**New Message Type**: `bot_wanted`

```json
{
  "type": "bot_wanted",
  "lobbyId": "uuid",
  "requestId": "uuid",
  "botSettings": {...},
  "rows": 20,
  "cols": 20
}
```

All other messages (`welcome`, `lobby_joined`, `game_start`, `move`, `turn_change`, etc.) are identical to human protocol.

## Questions?

- Architecture questions → See [BOT_HOSTER_PLAN_V2.md](BOT_HOSTER_PLAN_V2.md)
- Implementation questions → See specific task file
- Protocol questions → See [MULTIPLAYER.md](MULTIPLAYER.md)

## Next Steps

1. Review this roadmap
2. Read [BOT_HOSTER_PLAN_V2.md](BOT_HOSTER_PLAN_V2.md)
3. Start with [todo_task1.md](todo_task1.md)

Good luck! 🤖
