// Minimax AI for Virus Game with Alpha-Beta Pruning
// This AI uses game tree search to find optimal moves by looking ahead several turns

// Default search depth - controls how many moves ahead AI thinks
// Higher = smarter but slower. Recommended: 2-4
let aiDepth = 4;

// Time limit for AI search (milliseconds) - 0 means use fixed depth
let aiTimeLimit = 5000; // 5 seconds default

// Progress tracking
let aiProgressCurrent = 0;
let aiProgressTotal = 0;

// Transposition table for memoization (cache board evaluations)
let transpositionTable = new Map();
let ttHits = 0;
let ttMisses = 0;
let alphaBetaCutoffs = 0;

// Time management for iterative deepening
let searchStartTime = 0;
let searchTimeLimit = 0;

// Zobrist hashing for board states
const PIECE_TYPES = {
    '1': 0, '2': 1, '1-fortified': 2, '2-fortified': 3,
    '1-base': 4, '2-base': 5, 'killed': 6
};
const NUM_PIECE_TYPES = 7;
let zobristTable = [];
let zobristTableInitialized = false;

function initializeZobristTable() {
    // Only initialize if rows and cols are defined
    if (typeof rows === 'undefined' || typeof cols === 'undefined') {
        return;
    }

    if (zobristTableInitialized && zobristTable.length === rows) return;

    zobristTable = Array(rows).fill(null).map(() =>
        Array(cols).fill(null).map(() =>
            Array(NUM_PIECE_TYPES).fill(null).map(() =>
                // Use 2 32-bit numbers to simulate a 64-bit hash
                [Math.floor(Math.random() * 0xFFFFFFFF), Math.floor(Math.random() * 0xFFFFFFFF)]
            )
        )
    );
    zobristTableInitialized = true;
}

// AI Evaluation Coefficients (tunable in UI)
// Optimized to 5 key parameters for balanced evaluation
let aiCoeffs = {
    materialWeight: 100,     // Weight for material advantage (cells and fortifications)
    mobilityWeight: 50,      // Weight for having more available moves
    positionWeight: 30,      // Weight for strategic positioning (aggression + attacks)
    redundancyWeight: 40,    // Weight for network resilience (cells with multiple connections)
    cohesionWeight: 25       // Weight for territory cohesion (penalize gaps/holes)
};

// ============================================================================
// MAIN AI ENTRY POINT
// ============================================================================

function getAIMove() {
    if (gameOver || currentPlayer !== 2) {
        return null;
    }

    // Initialize Zobrist table on first run or if board size changes
    initializeZobristTable();

    // Reset progress tracking
    const possibleMoves = getAllValidMoves(board, 2);
    aiProgressCurrent = 0;
    aiProgressTotal = possibleMoves.length;
    updateAIProgress();

    // Clear transposition table for new search
    transpositionTable.clear();
    ttHits = 0;
    ttMisses = 0;
    alphaBetaCutoffs = 0;

    searchStartTime = performance.now();
    searchTimeLimit = aiTimeLimit > 0 ? aiTimeLimit : Infinity;

    let bestMove = null;
    let bestScore = -Infinity;
    let depthReached = 0;

    if (aiTimeLimit > 0) {
        // Iterative deepening: search progressively deeper until time runs out
        let lastDepthTime = 0;

        for (let depth = 1; depth <= 20; depth++) { // Max depth 20 as safety limit
            const timeElapsed = performance.now() - searchStartTime;
            const timeRemaining = searchTimeLimit - timeElapsed;

            // Conservative time estimate for next depth
            // Use 30% of remaining time as threshold
            if (depth > 1 && timeRemaining < searchTimeLimit * 0.3) {
                break;
            }

            // Also check if estimated next depth time exceeds remaining (with 5x multiplier for safety)
            const estimatedNextDepthTime = lastDepthTime * 5;
            if (depth > 1 && estimatedNextDepthTime > timeRemaining) {
                break;
            }

            const depthStartTime = performance.now();

            try {
                const result = minimax(board, depth, -Infinity, Infinity, true, true);

                // Only update best move if we completed this depth
                if (result && result.move) {
                    bestMove = result.move;
                    bestScore = result.score;
                    depthReached = depth;
                    lastDepthTime = performance.now() - depthStartTime;
                }

                // Hard stop if we exceeded time limit
                if (performance.now() - searchStartTime >= searchTimeLimit) {
                    break;
                }
            } catch (e) {
                // Time cutoff exception - use best move from previous depth
                break;
            }
        }
    } else {
        // Fixed depth search (original behavior)
        const result = minimax(board, aiDepth, -Infinity, Infinity, true, true);
        bestMove = result.move;
        bestScore = result.score;
        depthReached = aiDepth;
    }

    const duration = performance.now() - searchStartTime;
    const totalNodes = ttHits + ttMisses;
    console.log(`AI search: ${duration.toFixed(1)}ms | Depth: ${depthReached} | Nodes: ${totalNodes} | TT hits: ${ttHits} (${(ttHits/totalNodes*100).toFixed(1)}%) | AB cutoffs: ${alphaBetaCutoffs}`);

    // Hide progress indicator
    hideAIProgress();

    return bestMove;
}

function updateAIProgress() {
    const progressDiv = document.getElementById('ai-progress');
    const progressText = document.getElementById('ai-progress-text');

    if (progressDiv && progressText) {
        progressDiv.classList.remove('hidden');
        progressText.textContent = `${aiProgressCurrent}/${aiProgressTotal}`;
        console.log('AI Progress:', aiProgressCurrent, '/', aiProgressTotal);
    } else {
        console.warn('AI progress elements not found:', progressDiv, progressText);
    }
}

function hideAIProgress() {
    const progressDiv = document.getElementById('ai-progress');
    if (progressDiv) {
        progressDiv.classList.add('hidden');
    }
}

// ============================================================================
// MINIMAX ALGORITHM WITH ALPHA-BETA PRUNING
// ============================================================================

/**
 * Create a Zobrist hash key for a board state
 */
function hashBoard(boardState) {
    // Ensure Zobrist table is initialized
    if (!zobristTableInitialized || zobristTable.length === 0) {
        initializeZobristTable();
    }

    // Fallback to simple hash if Zobrist table isn't ready
    if (!zobristTableInitialized || zobristTable.length === 0) {
        // Simple hash: concatenate all cell values
        let hash = '';
        for (let r = 0; r < boardState.length; r++) {
            for (let c = 0; c < boardState[r].length; c++) {
                const cell = boardState[r][c];
                if (cell === null) {
                    hash += '0';
                } else if (typeof cell === 'number') {
                    hash += cell.toString();
                } else {
                    hash += cell; // string like "1-base"
                }
                hash += ',';
            }
        }
        return hash;
    }

    let hash = [0, 0]; // 64-bit hash as two 32-bit integers

    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            if (cell !== null) {
                const pieceType = PIECE_TYPES[cell];
                if (pieceType !== undefined && zobristTable[r] && zobristTable[r][c] && zobristTable[r][c][pieceType]) {
                    hash[0] ^= zobristTable[r][c][pieceType][0];
                    hash[1] ^= zobristTable[r][c][pieceType][1];
                }
            }
        }
    }
    return `${hash[0].toString(16)}-${hash[1].toString(16)}`;
}

/**
 * Minimax algorithm with alpha-beta pruning and transposition table
 * Explores the game tree to find optimal move by assuming both players play optimally
 *
 * @param {Array} boardState - Current board state
 * @param {number} depth - How many moves ahead to look (0 = evaluate current state)
 * @param {number} alpha - Best value maximizer can guarantee (for pruning)
 * @param {number} beta - Best value minimizer can guarantee (for pruning)
 * @param {boolean} isMaximizing - True if AI's turn (maximizing), false if opponent's turn (minimizing)
 * @returns {Object} {score: number, move: {row, col, score}}
 */
// Quick move scoring for move ordering (no deep evaluation)
function scoreMove(boardState, move, player) {
    const cellValue = boardState[move.row][move.col];
    const opponent = player === 1 ? 2 : 1;
    let score = 0;

    // 1. HIGHEST PRIORITY: Capturing opponent cells (fortifying)
    if (cellValue === opponent || String(cellValue).startsWith(opponent.toString())) {
        score += 1000;

        // Extra bonus if opponent cell is fortified (breaks their structure)
        if (String(cellValue).includes('fortified')) {
            score += 500;
        }
    }

    // 2. Count friendly and opponent neighbors for positional evaluation
    const directions = [[-1, 0], [1, 0], [0, -1], [0, 1]];
    let friendlyNeighbors = 0;
    let opponentNeighbors = 0;
    let emptyNeighbors = 0;

    for (const [dr, dc] of directions) {
        const nr = move.row + dr;
        const nc = move.col + dc;
        if (nr >= 0 && nr < rows && nc >= 0 && nc < cols) {
            const neighbor = boardState[nr][nc];
            if (neighbor && String(neighbor).startsWith(player.toString())) {
                friendlyNeighbors++;
            } else if (neighbor && String(neighbor).startsWith(opponent.toString())) {
                opponentNeighbors++;
            } else if (!neighbor) {
                emptyNeighbors++;
            }
        }
    }

    // 3. Reward moves with multiple friendly connections (stable expansion)
    score += friendlyNeighbors * 50;

    // 4. Reward moves that threaten opponent cells (attack opportunities)
    score += opponentNeighbors * 30;

    // 5. Reward expansion opportunities (empty neighbors for future growth)
    score += emptyNeighbors * 10;

    // 6. Distance to opponent base (aggression)
    const opponentBase = player === 1 ? player2Base : player1Base;
    const distToOpponentBase = Math.abs(move.row - opponentBase.row) + Math.abs(move.col - opponentBase.col);
    score -= distToOpponentBase * 3;

    // 7. Distance to own base (don't overextend)
    const ownBase = player === 1 ? player1Base : player2Base;
    const distToOwnBase = Math.abs(move.row - ownBase.row) + Math.abs(move.col - ownBase.col);
    if (distToOwnBase > 8) {
        score -= (distToOwnBase - 8) * 5; // Penalize overextension
    }

    return score;
}

function updateHash(oldHash, r, c, oldPiece, newPiece) {
    // Ensure Zobrist table is initialized
    if (!zobristTableInitialized || zobristTable.length === 0) {
        initializeZobristTable();
    }

    // If Zobrist table still isn't ready, fall back to regenerating the hash
    if (!zobristTableInitialized || zobristTable.length === 0) {
        // Return the old hash - incremental update not possible without Zobrist table
        return oldHash;
    }

    let [h1, h2] = oldHash.split('-').map(h => parseInt(h, 16));

    if (oldPiece !== null && zobristTable[r] && zobristTable[r][c]) {
        const oldPieceType = PIECE_TYPES[oldPiece];
        if (oldPieceType !== undefined && zobristTable[r][c][oldPieceType]) {
            h1 ^= zobristTable[r][c][oldPieceType][0];
            h2 ^= zobristTable[r][c][oldPieceType][1];
        }
    }

    if (newPiece !== null && zobristTable[r] && zobristTable[r][c]) {
        const newPieceType = PIECE_TYPES[newPiece];
        if (newPieceType !== undefined && zobristTable[r][c][newPieceType]) {
            h1 ^= zobristTable[r][c][newPieceType][0];
            h2 ^= zobristTable[r][c][newPieceType][1];
        }
    }

    return `${h1.toString(16)}-${h2.toString(16)}`;
}

function minimax(boardState, depth, alpha, beta, isMaximizing, isTopLevel = false, boardHash = null) {
    // Calculate hash at the top level
    if (isTopLevel) {
        boardHash = hashBoard(boardState);
    }
    const ttKey = `${boardHash}|${depth}|${isMaximizing}`;

    if (transpositionTable.has(ttKey)) {
        ttHits++;
        return transpositionTable.get(ttKey);
    }
    ttMisses++;

    // Base case: reached max depth or game over
    if (depth === 0) {
        const result = {
            score: evaluateBoard(boardState),
            move: null
        };
        transpositionTable.set(ttKey, result);
        return result;
    }

    const player = isMaximizing ? 2 : 1; // AI is player 2
    const possibleMoves = getAllValidMoves(boardState, player);

    // Move ordering: sort moves by heuristic score to try best moves first
    possibleMoves.sort((a, b) => {
        const scoreA = scoreMove(boardState, a, player);
        const scoreB = scoreMove(boardState, b, player);
        return isMaximizing ? (scoreB - scoreA) : (scoreA - scoreB);
    });

    // Terminal state: no moves available
    if (possibleMoves.length === 0) {
        const score = evaluateBoard(boardState);
        // Penalize losing positions, reward winning positions
        return {
            score: isMaximizing ? score - 10000 : score + 10000,
            move: null
        };
    }

    if (isMaximizing) {
        // AI's turn: maximize score
        let maxScore = -Infinity;
        let bestMove = possibleMoves[0];
        let moveIndex = 0;

        for (const move of possibleMoves) {
            // Update progress at top level BEFORE evaluating
            if (isTopLevel) {
                aiProgressCurrent = moveIndex + 1;
                updateAIProgress();
            }

            // Try this move
            const oldPiece = boardState[move.row][move.col];
            const newPiece = (oldPiece === null) ? player : `${player}-fortified`;
            const newBoard = applyMove(boardState, move.row, move.col, player);
            const newHash = updateHash(boardHash, move.row, move.col, oldPiece, newPiece);

            // Recursively evaluate this position
            const result = minimax(newBoard, depth - 1, alpha, beta, false, false, newHash);

            // Track best move
            if (result.score > maxScore) {
                maxScore = result.score;
                bestMove = move;
            }

            // Alpha-beta pruning
            alpha = Math.max(alpha, result.score);
            if (beta <= alpha) {
                alphaBetaCutoffs++;
                break; // Beta cutoff - opponent won't allow this branch
            }

            moveIndex++;
        }

        const result = { score: maxScore, move: bestMove };
        transpositionTable.set(ttKey, result);
        return result;

    } else {
        // Opponent's turn: minimize score
        let minScore = Infinity;
        let bestMove = possibleMoves[0];

        for (const move of possibleMoves) {
            // Try this move
            const oldPiece = boardState[move.row][move.col];
            const newPiece = (oldPiece === null) ? player : `${player}-fortified`;
            const newBoard = applyMove(boardState, move.row, move.col, player);
            const newHash = updateHash(boardHash, move.row, move.col, oldPiece, newPiece);

            // Recursively evaluate this position
            const result = minimax(newBoard, depth - 1, alpha, beta, true, false, newHash);

            // Track best move
            if (result.score < minScore) {
                minScore = result.score;
                bestMove = move;
            }

            // Alpha-beta pruning
            beta = Math.min(beta, result.score);
            if (beta <= alpha) {
                alphaBetaCutoffs++;
                break; // Alpha cutoff - AI won't allow this branch
            }
        }

        const result = { score: minScore, move: bestMove };
        transpositionTable.set(ttKey, result);
        return result;
    }
}

// ============================================================================
// BOARD EVALUATION FUNCTION
// ============================================================================

/**
 * Evaluates the board position from AI's perspective (player 2)
 * Positive scores favor AI, negative scores favor opponent
 *
 * Optimized evaluation with 5 components (all computed in single board pass):
 * 1. Material: cells and fortifications
 * 2. Mobility: available moves
 * 3. Strategic Position: aggression + attack opportunities
 * 4. Network Redundancy: resilient structure (cells with 2+ connections)
 * 5. Territory Cohesion: penalize gaps/holes in territory
 */
function evaluateBoard(boardState) {
    // === SINGLE PASS THROUGH BOARD ===
    let aiCells = 0;
    let opponentCells = 0;
    let aiFortified = 0;
    let opponentFortified = 0;
    let aiAttackOpportunities = 0;
    let opponentAttackOpportunities = 0;
    let aiAggression = 0;
    let opponentAggression = 0;
    let aiRedundantCells = 0;  // Cells with 2+ friendly neighbors (resilient)
    let opponentRedundantCells = 0;
    let aiCohesionPenalty = 0;  // Gaps/holes in territory
    let opponentCohesionPenalty = 0;

    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            const cellStr = String(cell);

            if (cellStr.startsWith('2')) {
                aiCells++;
                if (cellStr.includes('fortified')) aiFortified++;

                // Strategic position: distance to opponent base
                const distToOpponent = Math.abs(r - player1Base.row) + Math.abs(c - player1Base.col);
                aiAggression += (rows + cols - distToOpponent);

                // Our cells that opponent can attack
                const opponentNeighbors = countAdjacentCellsOnBoard(boardState, r, c, 1);
                if (opponentNeighbors > 0) {
                    opponentAttackOpportunities++;
                }

                // Fast redundancy: count cells with 2+ friendly neighbors
                const friendlyNeighbors = countAdjacentCellsOnBoard(boardState, r, c, 2);
                if (friendlyNeighbors >= 2) {
                    aiRedundantCells++;
                }

            } else if (cellStr.startsWith('1')) {
                opponentCells++;
                if (cellStr.includes('fortified')) opponentFortified++;

                // Count attack opportunities (opponent cells we can attack)
                const aiNeighbors = countAdjacentCellsOnBoard(boardState, r, c, 2);
                if (aiNeighbors > 0) {
                    aiAttackOpportunities++;
                }

                const distToAI = Math.abs(r - player2Base.row) + Math.abs(c - player2Base.col);
                opponentAggression += (rows + cols - distToAI);

                // Opponent redundancy
                const friendlyNeighbors = countAdjacentCellsOnBoard(boardState, r, c, 1);
                if (friendlyNeighbors >= 2) {
                    opponentRedundantCells++;
                }

            } else {
                // Empty or neutral cell - check for gaps/holes
                const aiFriendlyNeighbors = countAdjacentCellsOnBoard(boardState, r, c, 2);
                const opponentFriendlyNeighbors = countAdjacentCellsOnBoard(boardState, r, c, 1);

                // Gap in AI territory (empty cell surrounded by AI cells)
                if (aiFriendlyNeighbors >= 2) {
                    aiCohesionPenalty += aiFriendlyNeighbors;
                }

                // Gap in opponent territory
                if (opponentFriendlyNeighbors >= 2) {
                    opponentCohesionPenalty += opponentFriendlyNeighbors;
                }
            }
        }
    }

    // === 1. MATERIAL SCORE ===
    const materialScore = (aiCells * 10 + aiFortified * 20) -
                          (opponentCells * 10 + opponentFortified * 20);

    // === 2. MOBILITY SCORE ===
    const aiMoves = getAllValidMoves(boardState, 2).length;
    const opponentMoves = getAllValidMoves(boardState, 1).length;
    const mobilityScore = aiMoves - opponentMoves;

    // === 3. STRATEGIC POSITION SCORE ===
    const positionScore = (aiAggression - opponentAggression) +
                          (aiAttackOpportunities - opponentAttackOpportunities) * 5;

    // === 4. REDUNDANCY SCORE ===
    // Cells with 2+ neighbors are harder to cut off
    const redundancyScore = aiRedundantCells - opponentRedundantCells;

    // === 5. COHESION SCORE ===
    // Penalize gaps/holes in territory (fewer gaps = better)
    const cohesionScore = opponentCohesionPenalty - aiCohesionPenalty;

    // Combine scores with weights
    const totalScore = materialScore * aiCoeffs.materialWeight +
                       mobilityScore * aiCoeffs.mobilityWeight +
                       positionScore * aiCoeffs.positionWeight +
                       redundancyScore * aiCoeffs.redundancyWeight +
                       cohesionScore * aiCoeffs.cohesionWeight;

    return totalScore;
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Get all valid moves for a player on a given board state
 */
function getAllValidMoves(boardState, player) {
    const moves = [];

    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            if (isValidMoveOnBoard(boardState, r, c, player)) {
                moves.push({ row: r, col: c });
            }
        }
    }

    return moves;
}

/**
 * Check if a move is valid on a specific board state
 */
function isValidMoveOnBoard(boardState, row, col, player) {
    const cell = boardState[row][col];
    const opponent = player === 1 ? 2 : 1;

    // Cannot move on fortified, base, or neutral (killed) cells
    if (typeof cell === 'string' && (cell.includes('fortified') || cell.includes('base') || cell === 'killed')) {
        return false;
    }

    // Can only attack opponent's non-fortified cells or expand to empty cells
    if (cell !== null && !String(cell).startsWith(opponent.toString())) {
        return false;
    }

    // Check if adjacent to own territory
    if (!isAdjacentToPlayerOnBoard(boardState, row, col, player)) {
        return false;
    }

    // Check if the adjacent cell is connected to base
    // Find an adjacent cell that belongs to the player
    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const adjRow = row + i;
            const adjCol = col + j;

            if (adjRow >= 0 && adjRow < rows && adjCol >= 0 && adjCol < cols) {
                const adjCell = boardState[adjRow][adjCol];
                if (adjCell && String(adjCell).startsWith(player.toString())) {
                    // Check if this adjacent cell is connected to base
                    if (isConnectedToBaseOnBoard(boardState, adjRow, adjCol, player)) {
                        return true;
                    }
                }
            }
        }
    }

    return false;
}

/**
 * Check if a cell is connected to the player's base on a specific board
 */
function isConnectedToBaseOnBoard(boardState, startRow, startCol, player) {
    const base = player === 1 ? player1Base : player2Base;
    const visited = new Set();
    const stack = [{ row: startRow, col: startCol }];
    visited.add(`${startRow},${startCol}`);

    while (stack.length > 0) {
        const { row, col } = stack.pop();

        // Found the base
        if (row === base.row && col === base.col) {
            return true;
        }

        // Explore adjacent cells
        for (let i = -1; i <= 1; i++) {
            for (let j = -1; j <= 1; j++) {
                if (i === 0 && j === 0) continue;
                const newRow = row + i;
                const newCol = col + j;

                if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols && !visited.has(`${newRow},${newCol}`)) {
                    const cellValue = boardState[newRow][newCol];
                    if (cellValue && String(cellValue).startsWith(player.toString())) {
                        visited.add(`${newRow},${newCol}`);
                        stack.push({ row: newRow, col: newCol });
                    }
                }
            }
        }
    }
    return false;
}

/**
 * Check if a cell is adjacent to player's territory on a specific board
 */
function isAdjacentToPlayerOnBoard(boardState, row, col, player) {
    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const newRow = row + i;
            const newCol = col + j;

            if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols) {
                const adjCell = boardState[newRow][newCol];
                if (adjCell && String(adjCell).startsWith(player.toString())) {
                    return true;
                }
            }
        }
    }
    return false;
}

/**
 * Count adjacent cells belonging to a player on a specific board
 */
function countAdjacentCellsOnBoard(boardState, row, col, player) {
    let count = 0;

    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const newRow = row + i;
            const newCol = col + j;

            if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols) {
                const cell = boardState[newRow][newCol];
                if (cell && String(cell).startsWith(player.toString())) {
                    count++;
                }
            }
        }
    }

    return count;
}

/**
 * Apply a move to a board state (returns new board, doesn't modify original)
 */
function applyMove(boardState, row, col, player) {
    // Deep copy the board
    const newBoard = boardState.map(rowArr => rowArr.slice());

    const cell = newBoard[row][col];
    const opponent = player === 1 ? 2 : 1;

    if (cell === null) {
        // Expand to empty cell
        newBoard[row][col] = player;
    } else if (cell === opponent) {
        // Attack opponent's cell (fortify it)
        newBoard[row][col] = `${player}-fortified`;
    }

    return newBoard;
}

// ============================================================================
// AI TURN EXECUTION
// ============================================================================

function playAITurn() {
    if (gameOver || currentPlayer !== 2) {
        return;
    }

    if (movesLeft > 0) {
        // Show progress indicator before starting calculation
        setTimeout(() => {
            const move = getAIMove();

            if (move) {
                const cellValue = board[move.row][move.col];

                if (cellValue === null) {
                    board[move.row][move.col] = 2;
                } else if (cellValue === 1 || String(cellValue).startsWith('1')) {
                    board[move.row][move.col] = '2-fortified';
                }

                movesLeft--;
                renderBoard();
                updateStatus();

                if (!canMakeMove(2)) {
                    statusDisplay.textContent = 'Player 1 wins! Player 2 has no more moves.';
                    gameOver = true;
                    return;
                }
            }

            if (movesLeft > 0) {
                setTimeout(playAITurn, 500); // Make the next move after a short delay
            } else {
                endTurn();
            }
        }, 50); // Small delay to ensure UI updates
    }
}
