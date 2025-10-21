// Minimax AI for Virus Game with Alpha-Beta Pruning
// This AI uses game tree search to find optimal moves by looking ahead several turns

// Default search depth - controls how many moves ahead AI thinks
// Higher = smarter but slower. Recommended: 2-4
let aiDepth = 3;

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
let aiCoeffs = {
    cellValue: 8,            // Base value for a cell
    fortifiedValue: 25,      // Greatly incentivize capturing enemy cells
    mobilityValue: 3,        // Mobility is useful, but not critical
    aggressionValue: 2.5,    // Push towards the opponent's base
    connectionValue: 2,      // Encourage building connected structures, but allow for some spreading
    attackValue: 15,         // Highly reward opportunities to attack
    redundancyValue: 4,      // Build resilient networks, but not at the cost of aggression
    defensibilityValue: 2,   // A small bonus for defensive positions
    centerControlValue: 6,   // Prioritize controlling the center
    territoryCohesionValue: 3 // Penalize gaps more to create solid fronts
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
 * Evaluation criteria:
 * 1. Material: number of cells controlled
 * 2. Territory: connected territory size
 * 3. Mobility: number of available moves
 * 4. Position: strategic cell placement
 * 5. Threats: attack and defense opportunities
 */
function evaluateBoard(boardState) {
    let score = 0;

    // 1. MATERIAL ADVANTAGE
    // Count cells controlled by each player
    let aiCells = 0;
    let opponentCells = 0;
    let aiFortified = 0;
    let opponentFortified = 0;

    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            if (cell === 2 || String(cell).startsWith('2')) {
                aiCells++;
                if (String(cell).includes('fortified')) aiFortified++;
            } else if (cell === 1 || String(cell).startsWith('1')) {
                opponentCells++;
                if (String(cell).includes('fortified')) opponentFortified++;
            }
        }
    }

    // Material score: configurable points per cell type
    score += (aiCells * aiCoeffs.cellValue + aiFortified * aiCoeffs.fortifiedValue) -
             (opponentCells * aiCoeffs.cellValue + opponentFortified * aiCoeffs.fortifiedValue);

    // 2. MOBILITY ADVANTAGE
    // Count available moves for each player
    const aiMoves = getAllValidMoves(boardState, 2).length;
    const opponentMoves = getAllValidMoves(boardState, 1).length;

    // Mobility score: configurable points per available move
    score += (aiMoves - opponentMoves) * aiCoeffs.mobilityValue;

    // 3. POSITIONAL ADVANTAGE
    // Reward cells closer to opponent's base, penalize isolated cells
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];

            if (cell === 2 || String(cell).startsWith('2')) {
                // Reward aggressive positioning (closer to opponent base)
                const distToOpponent = Math.abs(r - player1Base.row) + Math.abs(c - player1Base.col);
                score += (rows + cols - distToOpponent) * aiCoeffs.aggressionValue;

                // Reward cells with multiple connections (less vulnerable)
                const connections = countAdjacentCellsOnBoard(boardState, r, c, 2);
                score += connections * aiCoeffs.connectionValue;

            } else if (cell === 1 || String(cell).startsWith('1')) {
                // Penalize opponent's aggressive positioning
                const distToAI = Math.abs(r - player2Base.row) + Math.abs(c - player2Base.col);
                score -= (rows + cols - distToAI) * aiCoeffs.aggressionValue;

                // Penalize opponent's well-connected cells
                const connections = countAdjacentCellsOnBoard(boardState, r, c, 1);
                score -= connections * aiCoeffs.connectionValue;
            }
        }
    }

    // 4. ATTACK OPPORTUNITIES
    // Reward positions where we can attack opponent cells
    let aiAttackOpportunities = 0;
    let opponentAttackOpportunities = 0;

    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];

            // Count opponent cells adjacent to our territory (attack opportunities)
            if (cell === 1 || String(cell).startsWith('1')) {
                if (countAdjacentCellsOnBoard(boardState, r, c, 2) > 0) {
                    aiAttackOpportunities++;
                }
            }

            // Count our cells adjacent to opponent territory (vulnerable positions)
            if (cell === 2 || String(cell).startsWith('2')) {
                if (countAdjacentCellsOnBoard(boardState, r, c, 1) > 0) {
                    opponentAttackOpportunities++;
                }
            }
        }
    }

    // Attack opportunities: configurable points each
    score += (aiAttackOpportunities - opponentAttackOpportunities) * aiCoeffs.attackValue;

    // 5. NETWORK REDUNDANCY
    // Reward resilient networks with multiple paths to base
    const aiRedundancy = calculateRedundancy(boardState, 2);
    const opponentRedundancy = calculateRedundancy(boardState, 1);
    score += (aiRedundancy - opponentRedundancy) * aiCoeffs.redundancyValue;

    // 6. DEFENSIBILITY
    // Reward structures where critical points are far from opponent
    // Encourages spread-out, harder-to-cut networks
    const aiDefensibility = calculateDefensibility(boardState, 2);
    const opponentDefensibility = calculateDefensibility(boardState, 1);
    score += (aiDefensibility - opponentDefensibility) * aiCoeffs.defensibilityValue;

    // 7. CENTER CONTROL
    // Reward controlling the center of the board
    const centerR = Math.floor(rows / 2);
    const centerC = Math.floor(cols / 2);
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            const distFromCenter = Math.abs(r - centerR) + Math.abs(c - centerC);
            const maxDist = centerR + centerC;
            const centerBonus = (maxDist - distFromCenter) * aiCoeffs.centerControlValue;

            if (String(cell).startsWith('2')) {
                score += centerBonus;
            } else if (String(cell).startsWith('1')) {
                score -= centerBonus;
            }
        }
    }

    // 8. TERRITORY COHESION
    // Penalize gaps and holes in territory
    const aiCohesion = calculateTerritoryCohesion(boardState, 2);
    const opponentCohesion = calculateTerritoryCohesion(boardState, 1);
    score += (aiCohesion - opponentCohesion) * aiCoeffs.territoryCohesionValue;


    return score;
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

/**
 * Calculate territory cohesion - penalizes gaps and holes
 * It works by counting empty or opponent cells adjacent to multiple friendly cells
 */
function calculateTerritoryCohesion(boardState, player) {
    let cohesionPenalty = 0;
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            if (cell === null || !String(cell).startsWith(player.toString())) {
                const friendlyNeighbors = countAdjacentCellsOnBoard(boardState, r, c, player);
                if (friendlyNeighbors > 1) {
                    // This is a gap or hole, penalize it
                    cohesionPenalty -= friendlyNeighbors * friendlyNeighbors;
                }
            }
        }
    }
    return cohesionPenalty;
}


/**
 * Calculate defensibility - minimum moves opponent needs to break our network
 * Finds critical cells (single points of failure) and measures distance from opponent's territory
 * Higher defensibility = harder for opponent to cut off our cells (more spread out, safer structure)
 */
function calculateDefensibility(boardState, player) {
    const opponent = player === 1 ? 2 : 1;
    const base = player === 1 ? player1Base : player2Base;
    let totalDefensibility = 0;

    // Find all non-base cells belonging to the player
    const playerCells = [];
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            const cellStr = String(cell);

            if (!cellStr.startsWith(player.toString())) continue;
            if (r === base.row && c === base.col) continue;

            playerCells.push({ row: r, col: c });
        }
    }

    // For each cell, check if it's critical (removing it disconnects some cells from base)
    for (const testCell of playerCells) {
        // Skip fortified cells - opponent can't easily capture them
        const cellValue = boardState[testCell.row][testCell.col];
        if (String(cellValue).includes('fortified')) continue;

        // Create a temporary board with this cell removed
        const tempBoard = boardState.map(row => row.slice());
        tempBoard[testCell.row][testCell.col] = null;

        // Check if any cells got disconnected
        let isCritical = false;
        for (const otherCell of playerCells) {
            if (otherCell.row === testCell.row && otherCell.col === testCell.col) continue;

            const cell = tempBoard[otherCell.row][otherCell.col];
            if (!cell) continue; // Skip if already removed

            // If this cell is now disconnected, the test cell is critical
            if (!isConnectedToBaseOnBoard(tempBoard, otherCell.row, otherCell.col, player)) {
                isCritical = true;
                break;
            }
        }

        // If critical, measure minimum distance from opponent's connected territory
        if (isCritical) {
            const distanceFromOpponent = minDistanceFromOpponentTerritory(
                boardState,
                testCell.row,
                testCell.col,
                opponent
            );

            // More distance = better defensibility
            totalDefensibility += distanceFromOpponent;
        }
    }

    return totalDefensibility;
}

/**
 * Find minimum Manhattan distance from a cell to opponent's connected territory
 */
function minDistanceFromOpponentTerritory(boardState, targetRow, targetCol, opponent) {
    let minDistance = rows + cols; // Max possible distance

    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            const cellStr = String(cell);

            // Check if this is opponent's cell connected to their base
            if (cellStr.startsWith(opponent.toString())) {
                if (isConnectedToBaseOnBoard(boardState, r, c, opponent)) {
                    const distance = Math.abs(targetRow - r) + Math.abs(targetCol - c);
                    minDistance = Math.min(minDistance, distance);
                }
            }
        }
    }

    return minDistance;
}

/**
 * Calculate network redundancy - how many cells can be removed while maintaining base connectivity
 * Higher redundancy = more resilient network with multiple paths to base
 */
function calculateRedundancy(boardState, player) {
    let redundantCells = 0;
    const base = player === 1 ? player1Base : player2Base;

    // Find all non-base, non-fortified cells belonging to the player
    const playerCells = [];
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            const cell = boardState[r][c];
            const cellStr = String(cell);

            // Skip if not player's cell, or if it's the base
            if (!cellStr.startsWith(player.toString())) continue;
            if (r === base.row && c === base.col) continue;

            // Non-fortified cells are candidates for redundancy check
            if (!cellStr.includes('fortified')) {
                playerCells.push({ row: r, col: c });
            }
        }
    }

    // For each cell, check if removing it still keeps the network connected
    for (const testCell of playerCells) {
        // Create a temporary board with this cell removed
        const tempBoard = boardState.map(row => row.slice());
        tempBoard[testCell.row][testCell.col] = null;

        // Check if all remaining cells are still connected to base
        let allConnected = true;
        for (let r = 0; r < rows; r++) {
            for (let c = 0; c < cols; c++) {
                const cell = tempBoard[r][c];
                const cellStr = String(cell);

                // Skip non-player cells and the base itself
                if (!cellStr.startsWith(player.toString())) continue;
                if (r === base.row && c === base.col) continue;

                // Check if this cell is still connected to base
                if (!isConnectedToBaseOnBoard(tempBoard, r, c, player)) {
                    allConnected = false;
                    break;
                }
            }
            if (!allConnected) break;
        }

        // If removing this cell keeps network connected, it's redundant
        if (allConnected) {
            redundantCells++;
        }
    }

    return redundantCells;
}

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

    // Cannot move on fortified or base cells
    if (typeof cell === 'string' && (cell.includes('fortified') || cell.includes('base'))) {
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
