// Minimax AI for Virus Game with Alpha-Beta Pruning
// This AI uses game tree search to find optimal moves by looking ahead several turns

// Default search depth - controls how many moves ahead AI thinks
// Higher = smarter but slower. Recommended: 2-4
let aiDepth = 3;

// Progress tracking
let aiProgressCurrent = 0;
let aiProgressTotal = 0;

// AI Evaluation Coefficients (tunable in UI)
let aiCoeffs = {
    cellValue: 10,           // Points per regular cell
    fortifiedValue: 15,      // Extra points per fortified cell
    mobilityValue: 5,        // Points per available move
    aggressionValue: 1,      // Points per step closer to opponent (rows+cols-distance)
    connectionValue: 3,      // Points per adjacent friendly cell
    attackValue: 8,          // Points per attack opportunity
    redundancyValue: 5       // Points per redundant connection (cells that can be lost while maintaining base connectivity)
};

// ============================================================================
// MAIN AI ENTRY POINT
// ============================================================================

function getAIMove() {
    if (gameOver || currentPlayer !== 2) {
        return null;
    }

    // Reset progress tracking
    const possibleMoves = getAllValidMoves(board, 2);
    aiProgressCurrent = 0;
    aiProgressTotal = possibleMoves.length;
    updateAIProgress();

    // Use minimax to find the best move
    const result = minimax(board, aiDepth, -Infinity, Infinity, true, true);

    // Hide progress indicator
    hideAIProgress();

    return result.move;
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
 * Minimax algorithm with alpha-beta pruning
 * Explores the game tree to find optimal move by assuming both players play optimally
 *
 * @param {Array} boardState - Current board state
 * @param {number} depth - How many moves ahead to look (0 = evaluate current state)
 * @param {number} alpha - Best value maximizer can guarantee (for pruning)
 * @param {number} beta - Best value minimizer can guarantee (for pruning)
 * @param {boolean} isMaximizing - True if AI's turn (maximizing), false if opponent's turn (minimizing)
 * @returns {Object} {score: number, move: {row, col, score}}
 */
function minimax(boardState, depth, alpha, beta, isMaximizing, isTopLevel = false) {
    // Base case: reached max depth or game over
    if (depth === 0) {
        return {
            score: evaluateBoard(boardState),
            move: null
        };
    }

    const player = isMaximizing ? 2 : 1; // AI is player 2
    const possibleMoves = getAllValidMoves(boardState, player);

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
            const newBoard = applyMove(boardState, move.row, move.col, player);

            // Recursively evaluate this position
            const result = minimax(newBoard, depth - 1, alpha, beta, false, false);

            // Track best move
            if (result.score > maxScore) {
                maxScore = result.score;
                bestMove = move;
            }

            // Alpha-beta pruning
            alpha = Math.max(alpha, result.score);
            if (beta <= alpha) {
                break; // Beta cutoff - opponent won't allow this branch
            }

            moveIndex++;
        }

        return { score: maxScore, move: bestMove };

    } else {
        // Opponent's turn: minimize score
        let minScore = Infinity;
        let bestMove = possibleMoves[0];

        for (const move of possibleMoves) {
            // Try this move
            const newBoard = applyMove(boardState, move.row, move.col, player);

            // Recursively evaluate this position
            const result = minimax(newBoard, depth - 1, alpha, beta, true);

            // Track best move
            if (result.score < minScore) {
                minScore = result.score;
                bestMove = move;
            }

            // Alpha-beta pruning
            beta = Math.min(beta, result.score);
            if (beta <= alpha) {
                break; // Alpha cutoff - AI won't allow this branch
            }
        }

        return { score: minScore, move: bestMove };
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

    return score;
}

// ============================================================================
// HELPER FUNCTIONS
// ============================================================================

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
