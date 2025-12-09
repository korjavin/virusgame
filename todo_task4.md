# Task 4: Bot AI Integration - Make Bots Play

## Context

Task 3 implemented bots that can connect, join lobbies, and track game state. But they don't actually play - they receive `turn_change` messages but don't calculate or send moves.

This task integrates the AI engine (from `backend/bot.go`) into the bot client so bots can actually play the game.

## Prerequisites

- Task 1 completed (Backend broadcasts bot_wanted)
- Task 2 completed (Docker setup)
- Task 3 completed (Bot-hoster service with bot pool)

## Related Files

- `backend/bot.go` - AI engine with minimax algorithm
- `backend/cmd/bot-hoster/bot_client.go` - Bot client (from Task 3)
- `backend/cmd/bot-hoster/ai_engine.go` - NEW: Wrapper for AI logic
- `backend/types.go` - Game types

## Goal

Enable bots to calculate moves using AI and send them to the backend, completing the bot gameplay loop.

## Acceptance Criteria

1. ✅ Bot receives `turn_change` message for its turn
2. ✅ Bot calculates best move using AI (minimax algorithm)
3. ✅ Bot sends `move` message with calculated row/col
4. ✅ Backend validates and applies move
5. ✅ Bot continues playing until game ends
6. ✅ Bot uses BotSettings from lobby (AI coefficients)
7. ✅ Multiple moves per turn (3 moves) work correctly
8. ✅ AI respects search depth from settings
9. ✅ Bot handles "no valid moves" gracefully

## Implementation Steps

### Step 1: Create AI Engine Wrapper

The AI logic already exists in `backend/bot.go`, but it's tightly coupled to the Hub. We need to extract and adapt it for use in the bot client.

**File**: `backend/cmd/bot-hoster/ai_engine.go`

```go
package main

import (
    "fmt"
    "log"
    "math"
    "sort"
    "strings"
    "sync"
)

// AIEngine handles bot move calculations
type AIEngine struct {
    settings *BotSettings
    transTable *TranspositionTable
}

// TranspositionTable caches board evaluations
type TranspositionTable struct {
    table map[string]TranspositionEntry
    mu    sync.RWMutex
}

type TranspositionEntry struct {
    Score float64
    Depth int
    Flag  int
}

const (
    exactScore = iota
    lowerBound
    upperBound
)

func NewAIEngine(settings *BotSettings) *AIEngine {
    return &AIEngine{
        settings:   settings,
        transTable: NewTranspositionTable(),
    }
}

func NewTranspositionTable() *TranspositionTable {
    return &TranspositionTable{
        table: make(map[string]TranspositionEntry),
    }
}

// CalculateMove returns the best move for the given game state
// Returns (row, col, ok) where ok is false if no valid moves exist
func (ai *AIEngine) CalculateMove(state *GameState, player int) (int, int, bool) {
    depth := ai.settings.SearchDepth
    if depth <= 0 {
        depth = 3 // default
    }

    log.Printf("[AI] Calculating move for player %d (depth: %d)", player, depth)

    // Get all valid moves
    validMoves := ai.getAllValidMoves(state, player)
    if len(validMoves) == 0 {
        return 0, 0, false
    }

    // Use minimax to find best move
    bestMove := ai.findBestMoveWithMinimax(state, validMoves, player, depth)

    log.Printf("[AI] Selected move: (%d, %d) with score %.2f",
        bestMove.Row, bestMove.Col, bestMove.Score)

    return bestMove.Row, bestMove.Col, true
}

// GameState represents the current state of the game
type GameState struct {
    Board       [][]interface{}
    Rows        int
    Cols        int
    PlayerBases [4]CellPos
    Players     []GamePlayerInfo
}

type Move struct {
    Row   int
    Col   int
    Score float64
}

// Note: The rest of this file contains the minimax algorithm
// Copy the following functions from backend/bot.go and adapt them:
// - getAllValidMoves()
// - findBestMoveWithMinimax()
// - minimax()
// - evaluateBoard()
// - scoreMoveQuick()
// - isValidMove()
// - isAdjacentAndConnected()
// - isConnectedToBase()
// - copyBoard()
// - applyMoveToBoard()
// - hashBoard()
// - All helper functions

// For brevity, I'll show the structure - you'll copy the actual implementations

func (ai *AIEngine) getAllValidMoves(state *GameState, player int) []Move {
    // TODO: Copy from backend/bot.go getAllBotMoves()
    // Adapt to work with GameState instead of Game
    var moves []Move
    for row := 0; row < state.Rows; row++ {
        for col := 0; col < state.Cols; col++ {
            if ai.isValidMove(state, row, col, player) {
                moves = append(moves, Move{Row: row, Col: col})
            }
        }
    }
    return moves
}

func (ai *AIEngine) isValidMove(state *GameState, row, col, player int) bool {
    // Copy logic from backend/bot.go isValidMoveOnBoard()
    cell := state.Board[row][col]
    cellStr := fmt.Sprintf("%v", cell)

    // Cannot move on fortified or base cells
    if cell != nil {
        if strings.HasSuffix(cellStr, "-fortified") || strings.HasSuffix(cellStr, "-base") {
            return false
        }
    }

    // Can only attack opponent's non-fortified cells or expand to empty cells
    if cell != nil {
        isOpponent := false
        for _, p := range state.Players {
            if p.PlayerIndex+1 != player && p.IsActive && len(cellStr) > 0 && cellStr[0] == byte('0'+p.PlayerIndex+1) {
                isOpponent = true
                break
            }
        }
        if !isOpponent {
            return false
        }
    }

    // Must be adjacent to own territory and connected to base
    return ai.isAdjacentAndConnected(state, row, col, player)
}

func (ai *AIEngine) isAdjacentAndConnected(state *GameState, row, col, player int) bool {
    // Copy from backend/bot.go isAdjacentAndConnectedOnBoard()
    // Check all 8 neighbors for friendly cells that are connected to base
    for i := -1; i <= 1; i++ {
        for j := -1; j <= 1; j++ {
            if i == 0 && j == 0 {
                continue
            }
            adjRow := row + i
            adjCol := col + j
            if adjRow >= 0 && adjRow < state.Rows && adjCol >= 0 && adjCol < state.Cols {
                adjCell := state.Board[adjRow][adjCol]
                adjStr := fmt.Sprintf("%v", adjCell)
                if adjCell != nil && len(adjStr) > 0 && adjStr[0] == byte('0'+player) {
                    if ai.isConnectedToBase(state, adjRow, adjCol, player) {
                        return true
                    }
                }
            }
        }
    }
    return false
}

func (ai *AIEngine) isConnectedToBase(state *GameState, startRow, startCol, player int) bool {
    // Copy from backend/bot.go isConnectedToBaseOnBoard()
    // BFS to check if cell is connected to base
    base := state.PlayerBases[player-1]
    visited := make(map[string]bool)
    stack := []struct{ row, col int }{{startRow, startCol}}
    visited[fmt.Sprintf("%d,%d", startRow, startCol)] = true

    for len(stack) > 0 {
        curr := stack[len(stack)-1]
        stack = stack[:len(stack)-1]

        if curr.row == base.Row && curr.col == base.Col {
            return true
        }

        for i := -1; i <= 1; i++ {
            for j := -1; j <= 1; j++ {
                if i == 0 && j == 0 {
                    continue
                }
                newRow := curr.row + i
                newCol := curr.col + j
                key := fmt.Sprintf("%d,%d", newRow, newCol)

                if newRow >= 0 && newRow < state.Rows && newCol >= 0 && newCol < state.Cols && !visited[key] {
                    cell := state.Board[newRow][newCol]
                    cellStr := fmt.Sprintf("%v", cell)
                    if cell != nil && len(cellStr) > 0 && cellStr[0] == byte('0'+player) {
                        visited[key] = true
                        stack = append(stack, struct{ row, col int }{newRow, newCol})
                    }
                }
            }
        }
    }
    return false
}

func (ai *AIEngine) findBestMoveWithMinimax(state *GameState, moves []Move, player int, depth int) Move {
    // Copy from backend/bot.go findBestMoveWithMinimax()
    // Sort moves by heuristic for better pruning
    for i := range moves {
        moves[i].Score = ai.scoreMoveQuick(state, moves[i], player)
    }
    sort.Slice(moves, func(i, j int) bool {
        return moves[i].Score > moves[j].Score
    })

    // Limit moves to consider
    maxMoves := 20
    if len(moves) > maxMoves {
        moves = moves[:maxMoves]
    }

    bestMove := moves[0]
    bestScore := math.Inf(-1)
    alpha := math.Inf(-1)
    beta := math.Inf(1)

    for _, move := range moves {
        newBoard := ai.copyBoard(state.Board)
        ai.applyMoveToBoard(newBoard, move.Row, move.Col, player)

        newState := &GameState{
            Board:       newBoard,
            Rows:        state.Rows,
            Cols:        state.Cols,
            PlayerBases: state.PlayerBases,
            Players:     state.Players,
        }

        score := ai.minimax(newState, depth-1, alpha, beta, false, player)

        if score > bestScore {
            bestScore = score
            bestMove = move
            bestMove.Score = bestScore
        }

        alpha = math.Max(alpha, score)
        if beta <= alpha {
            break
        }
    }

    return bestMove
}

func (ai *AIEngine) minimax(state *GameState, depth int, alpha, beta float64, isMaximizing bool, aiPlayer int) float64 {
    // Copy minimax implementation from backend/bot.go
    // This is the core algorithm - copy the full implementation

    // Check transposition table
    boardHash := ai.hashBoard(state.Board, aiPlayer)
    if entry, exists := ai.transTable.Get(boardHash); exists && entry.Depth >= depth {
        if entry.Flag == exactScore {
            return entry.Score
        } else if entry.Flag == lowerBound {
            alpha = math.Max(alpha, entry.Score)
        } else if entry.Flag == upperBound {
            beta = math.Min(beta, entry.Score)
        }
        if alpha >= beta {
            return entry.Score
        }
    }

    // Base case: reached max depth
    if depth == 0 {
        score := ai.evaluateBoard(state, aiPlayer)
        ai.transTable.Put(boardHash, TranspositionEntry{
            Score: score,
            Depth: depth,
            Flag:  exactScore,
        })
        return score
    }

    // ... rest of minimax logic (copy from backend/bot.go)

    return 0.0 // Placeholder
}

func (ai *AIEngine) evaluateBoard(state *GameState, aiPlayer int) float64 {
    // Copy from backend/bot.go evaluateBoard()
    // This calculates the board score using the AI coefficients

    // Single pass through board to collect metrics
    aiCells := 0
    opponentCells := 0
    // ... rest of evaluation logic

    materialScore := 0.0
    mobilityScore := 0.0
    positionScore := 0.0
    redundancyScore := 0.0
    cohesionScore := 0.0

    // Combine with weights
    totalScore := materialScore*ai.settings.MaterialWeight +
        mobilityScore*ai.settings.MobilityWeight +
        positionScore*ai.settings.PositionWeight +
        redundancyScore*ai.settings.RedundancyWeight +
        cohesionScore*ai.settings.CohesionWeight

    return totalScore
}

func (ai *AIEngine) scoreMoveQuick(state *GameState, move Move, player int) float64 {
    // Copy from backend/bot.go scoreMoveQuick()
    // Quick heuristic for move ordering
    return 0.0 // Placeholder
}

func (ai *AIEngine) copyBoard(board [][]interface{}) [][]interface{} {
    newBoard := make([][]interface{}, len(board))
    for i := range board {
        newBoard[i] = make([]interface{}, len(board[i]))
        copy(newBoard[i], board[i])
    }
    return newBoard
}

func (ai *AIEngine) applyMoveToBoard(board [][]interface{}, row, col, player int) {
    cell := board[row][col]
    if cell == nil {
        board[row][col] = player
    } else {
        board[row][col] = fmt.Sprintf("%d-fortified", player)
    }
}

func (ai *AIEngine) hashBoard(board [][]interface{}, player int) string {
    var key strings.Builder
    key.WriteString(fmt.Sprintf("P%d:", player))
    for r := range board {
        for c := range board[r] {
            if board[r][c] == nil {
                key.WriteString("_")
            } else {
                key.WriteString(fmt.Sprintf("%v", board[r][c]))
            }
            key.WriteString(",")
        }
    }
    return key.String()
}

// TranspositionTable methods
func (tt *TranspositionTable) Get(key string) (TranspositionEntry, bool) {
    tt.mu.RLock()
    defer tt.mu.RUnlock()
    entry, exists := tt.table[key]
    return entry, exists
}

func (tt *TranspositionTable) Put(key string, entry TranspositionEntry) {
    tt.mu.Lock()
    defer tt.mu.Unlock()
    tt.table[key] = entry
}
```

**Important**: The above is a template. You need to **copy the actual implementations** from `backend/bot.go` functions:
- Lines 77-105: `calculateBotMove` → adapt to `CalculateMove`
- Lines 144-185: `findBestMoveWithMinimax`
- Lines 188-343: `minimax`
- Lines 347-480: `evaluateBoard`
- Lines 483-575: `scoreMoveQuick`
- Lines 579-833: All helper functions

### Step 2: Integrate AI into Bot Client

**File**: `backend/cmd/bot-hoster/bot_client.go`

Update `handleGameStart` to initialize AI engine:

```go
func (b *Bot) handleGameStart(msg *Message) {
    b.mu.Lock()
    b.State = BotInGame
    b.CurrentGame = msg.GameID
    b.YourPlayer = msg.YourPlayer
    b.Rows = msg.Rows
    b.Cols = msg.Cols
    b.GamePlayers = msg.GamePlayers

    // Initialize board
    b.Board = make([][]interface{}, b.Rows)
    for i := range b.Board {
        b.Board[i] = make([]interface{}, b.Cols)
    }

    // Initialize player bases
    // TODO: Backend should send this in message, for now assume standard
    b.PlayerBases[0] = CellPos{Row: 0, Col: 0}
    b.PlayerBases[1] = CellPos{Row: b.Rows - 1, Col: b.Cols - 1}
    b.PlayerBases[2] = CellPos{Row: 0, Col: b.Cols - 1}
    b.PlayerBases[3] = CellPos{Row: b.Rows - 1, Col: 0}

    // Place bases on board
    b.Board[b.PlayerBases[0].Row][b.PlayerBases[0].Col] = "1-base"
    b.Board[b.PlayerBases[1].Row][b.PlayerBases[1].Col] = "2-base"
    if len(b.GamePlayers) > 2 {
        b.Board[b.PlayerBases[2].Row][b.PlayerBases[2].Col] = "3-base"
    }
    if len(b.GamePlayers) > 3 {
        b.Board[b.PlayerBases[3].Row][b.PlayerBases[3].Col] = "4-base"
    }

    // NEW: Initialize AI engine with bot settings
    if b.BotSettings != nil {
        b.AIEngine = NewAIEngine(b.BotSettings)
    } else {
        // Use defaults
        b.AIEngine = NewAIEngine(&BotSettings{
            MaterialWeight:   100.0,
            MobilityWeight:   50.0,
            PositionWeight:   30.0,
            RedundancyWeight: 40.0,
            CohesionWeight:   25.0,
            SearchDepth:      5,
        })
    }

    b.mu.Unlock()

    log.Printf("[Bot %s] Game started as player %d in game %s (AI ready)",
        b.Username, b.YourPlayer, b.CurrentGame)
}
```

Update `handleTurnChange` to calculate and send move:

```go
func (b *Bot) handleTurnChange(msg *Message) {
    b.mu.RLock()
    isMyTurn := msg.Player == b.YourPlayer
    gameID := b.CurrentGame
    b.mu.RUnlock()

    if isMyTurn {
        log.Printf("[Bot %s] My turn! Calculating move...", b.Username)
        go b.calculateAndSendMove(gameID)
    }
}
```

Add new method `calculateAndSendMove`:

```go
// calculateAndSendMove runs AI to find best move and sends it
func (b *Bot) calculateAndSendMove(gameID string) {
    b.mu.RLock()

    // Create game state snapshot
    state := &GameState{
        Board:       b.copyBoardLocal(b.Board),
        Rows:        b.Rows,
        Cols:        b.Cols,
        PlayerBases: b.PlayerBases,
        Players:     b.GamePlayers,
    }
    player := b.YourPlayer
    aiEngine := b.AIEngine

    b.mu.RUnlock()

    if aiEngine == nil {
        log.Printf("[Bot %s] ERROR: AI engine not initialized!", b.Username)
        return
    }

    // Calculate move (may take 500ms - 2s)
    row, col, ok := aiEngine.CalculateMove(state, player)

    if !ok {
        log.Printf("[Bot %s] No valid moves available!", b.Username)
        // TODO: Could send resign message here
        return
    }

    // Send move
    rowPtr := row
    colPtr := col
    msg := Message{
        Type:   "move",
        GameID: gameID,
        Row:    &rowPtr,
        Col:    &colPtr,
    }

    b.sendMessage(&msg)

    log.Printf("[Bot %s] Sent move: (%d, %d)", b.Username, row, col)
}

func (b *Bot) copyBoardLocal(board [][]interface{}) [][]interface{} {
    newBoard := make([][]interface{}, len(board))
    for i := range board {
        newBoard[i] = make([]interface{}, len(board[i]))
        copy(newBoard[i], board[i])
    }
    return newBoard
}
```

### Step 3: Add AI Engine Field to Bot Struct

**File**: `backend/cmd/bot-hoster/bot_client.go`

Update Bot struct:

```go
type Bot struct {
    ID           string
    Username     string
    UserID       string
    WS           *websocket.Conn
    State        BotState
    Manager      *BotManager
    BackendURL   string

    // Current activity
    CurrentLobby string
    CurrentGame  string
    YourPlayer   int
    BotSettings  *BotSettings

    // Game state
    Board        [][]interface{}
    GamePlayers  []GamePlayerInfo
    PlayerBases  [4]CellPos
    Rows         int
    Cols         int

    // AI
    AIEngine     *AIEngine  // NEW

    // Communication channels
    send         chan []byte
    done         chan bool

    // Synchronization
    mu           sync.RWMutex
}
```

## Testing Steps

### Test 1: Bot Makes First Move

```bash
# 1. Start backend
cd backend && go run .

# 2. Start bot-hoster
export BACKEND_URL=ws://localhost:8080/ws
export BOT_POOL_SIZE=1
go run ./cmd/bot-hoster

# 3. Create lobby, add bot, add yourself, start game

# Expected:
# - Game starts
# - Player 1's turn (human or bot)
# - If bot is player 1:
#   [Bot AI-BraveOctopus] My turn! Calculating move...
#   [AI] Calculating move for player 1 (depth: 5)
#   [AI] Selected move: (1, 0) with score 450.2
#   [Bot AI-BraveOctopus] Sent move: (1, 0)
# - Board updates with bot's move
```

### Test 2: Bot Completes Full Turn (3 Moves)

```bash
# Start game with bot as player 1

# Expected log sequence:
# Turn starts (3 moves left)
# [Bot] Calculating move...
# [Bot] Sent move: (1, 0)
# Backend: move_made, 2 moves left
# [Bot] Calculating move...
# [Bot] Sent move: (1, 1)
# Backend: move_made, 1 move left
# [Bot] Calculating move...
# [Bot] Sent move: (2, 0)
# Backend: move_made, 0 moves left
# Backend: turn_change to player 2
```

### Test 3: Bot Plays Full Game

```bash
# Create 2-player game: Human vs Bot
# Let the game play out

# Expected:
# - Bot makes moves on its turns
# - Human makes moves on their turns
# - Game continues until win/loss
# - Bot returns to idle after game ends
```

### Test 4: Multiple Bots in Same Game

```bash
# Create 4-player lobby
# Add 3 bots
# Add yourself
# Start game

# Expected:
# - All 3 bots calculate moves on their turns
# - Each bot uses its own AI engine
# - No interference between bots
# - Game progresses normally
```

### Test 5: Custom AI Settings

```javascript
// In browser console, before clicking "Add Bot":
aiCoeffs.materialWeight = 200.0; // Make bot value captures more
aiCoeffs.searchDepth = 3; // Make bot faster but weaker

// Then click "Add Bot"

// Expected:
# Bot uses custom settings
# [AI] Calculating move for player 2 (depth: 3)
# Bot plays with different strategy (more aggressive captures)
```

### Test 6: Performance Test

```bash
# Create game, add bot
# Measure time between turn_change and move sent

# Expected:
# - Depth 3: ~200-500ms per move
# - Depth 5: ~500-2000ms per move
# - No timeouts or deadlocks
```

## Edge Cases

1. **No valid moves**: Bot surrounded, can't move
   - Expected: Bot logs "No valid moves available"
   - Game should handle (auto-resign or skip turn)

2. **Disconnection during calculation**: Bot loses connection while thinking
   - Expected: Calculation completes but send fails
   - Bot reconnects, game already moved on

3. **Very long calculation**: High depth + large board
   - Expected: May take 5-10 seconds
   - Backend should wait (move timeout handled by backend)

## Dependencies

**Blocked by**:
- Task 3 (Bot client implementation)

**Blocks**:
- Task 5 (Bot guide - shows working bot example)

## Estimated Time

**4-6 hours**

- 2 hours: Copy and adapt AI engine
- 1 hour: Integrate into bot client
- 1 hour: Testing full game
- 1 hour: Debugging and optimization
- 1 hour: Documentation

## Success Validation

After this task, bots should:

```bash
# 1. Calculate moves
✓ Logs show "Calculating move for player N"

# 2. Send moves
✓ Backend receives move message
✓ Board updates with bot's move

# 3. Complete turns
✓ Bot makes 3 moves per turn
✓ Turn changes to next player

# 4. Play full game
✓ Bot plays until game ends
✓ Bot wins or loses appropriately

# 5. Use custom settings
✓ Bot respects AI coefficients from frontend
✓ Different search depths work
```

## Performance Targets

- **Move calculation**: < 2 seconds (depth 5, 20x20 board)
- **Memory per bot**: < 100 MB during game
- **CPU usage**: Spikes during calculation, idle otherwise
- **Concurrent bots**: 10 bots calculating simultaneously should work

## Notes

- The AI engine is **computationally expensive** - this is why we're offloading to separate service!
- Each bot has its own AI engine instance (no sharing)
- Transposition table is per-bot (cleared between games)
- Move calculation runs in goroutine to avoid blocking bot's message loop

## Related Documentation

- `backend/bot.go` - Original AI implementation
- `BOT_HOSTER_PLAN_V2.md` - Architecture overview
- AI algorithm explanation: Minimax with alpha-beta pruning
