// Smart AI for Virus Game
// Strategy:
// 1. Expand territory to create multiple branches
// 2. Attack opponent when advantageous
// 3. Avoid getting surrounded/blocked
// 4. Prioritize moves that keep options open

function evaluateMove(row, col, player) {
    // Score a move based on strategic value
    let score = 0;
    const opponent = player === 1 ? 2 : 1;
    const cellValue = board[row][col];

    // 1. Attack moves are valuable
    if (cellValue === opponent || String(cellValue).startsWith(opponent)) {
        score += 100; // High priority for attacks

        // Extra value for attacks that fortify and block opponent
        const adjacentOpponentCells = countAdjacentCells(row, col, opponent);
        score += adjacentOpponentCells * 20; // More value if blocking enemy expansion
    }

    // 2. Expansion moves that create branches
    if (cellValue === null) {
        // Count how many empty cells this opens up
        const newOpportunities = countAdjacentEmpty(row, col);
        score += newOpportunities * 15;

        // Bonus for moves that spread out (not just clustering)
        const distanceFromBase = getDistanceFromBase(row, col, player);
        score += distanceFromBase * 5; // Encourage spreading

        // Penalty for moves too close to existing cells (avoid clustering)
        const adjacentOwn = countAdjacentCells(row, col, player);
        if (adjacentOwn > 2) {
            score -= 10; // Small penalty for clustering
        }
    }

    // 3. Avoid moves that can be easily surrounded
    const surroundingDanger = evaluateSurroundingDanger(row, col, player);
    score -= surroundingDanger;

    // 4. Prefer moves closer to opponent's base (aggressive)
    const opponentBase = player === 1 ? player2Base : player1Base;
    const distToOpponentBase = Math.abs(row - opponentBase.row) + Math.abs(col - opponentBase.col);
    score += (rows + cols - distToOpponentBase) * 2;

    // 5. Bonus for creating parallel branches (multiple paths)
    if (cellValue === null && hasMultipleConnectionPaths(row, col, player)) {
        score += 25;
    }

    return score;
}

function countAdjacentCells(row, col, player) {
    let count = 0;
    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const newRow = row + i;
            const newCol = col + j;
            if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols) {
                const cell = board[newRow][newCol];
                if (cell && String(cell).startsWith(player)) {
                    count++;
                }
            }
        }
    }
    return count;
}

function countAdjacentEmpty(row, col) {
    let count = 0;
    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const newRow = row + i;
            const newCol = col + j;
            if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols) {
                if (board[newRow][newCol] === null) {
                    count++;
                }
            }
        }
    }
    return count;
}

function getDistanceFromBase(row, col, player) {
    const base = player === 1 ? player1Base : player2Base;
    return Math.abs(row - base.row) + Math.abs(col - base.col);
}

function evaluateSurroundingDanger(row, col, player) {
    const opponent = player === 1 ? 2 : 1;
    let danger = 0;

    // Count how many opponent cells surround this position
    const opponentAdjacent = countAdjacentCells(row, col, opponent);

    // If surrounded by enemies, this is dangerous
    if (opponentAdjacent >= 5) {
        danger += 50; // High danger
    } else if (opponentAdjacent >= 3) {
        danger += 20; // Medium danger
    }

    return danger;
}

function hasMultipleConnectionPaths(row, col, player) {
    // Check if placing here creates multiple independent paths to base
    let connectionPoints = 0;

    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const adjRow = row + i;
            const adjCol = col + j;

            if (adjRow >= 0 && adjRow < rows && adjCol >= 0 && adjCol < cols) {
                const cell = board[adjRow][adjCol];
                if (cell && String(cell).startsWith(player)) {
                    connectionPoints++;
                }
            }
        }
    }

    // If we can connect from multiple directions, it's a strong position
    return connectionPoints >= 2;
}

function getAIMove() {
    const possibleMoves = [];

    // Get all valid moves with their scores
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            if (isValidMove(r, c, 2)) {
                const score = evaluateMove(r, c, 2);
                possibleMoves.push({ row: r, col: c, score: score });
            }
        }
    }

    if (possibleMoves.length === 0) {
        return null;
    }

    // Sort moves by score (best first)
    possibleMoves.sort((a, b) => b.score - a.score);

    // Take top 3 moves and pick randomly among them (adds some variety)
    const topMoves = possibleMoves.slice(0, Math.min(3, possibleMoves.length));
    const selectedMove = topMoves[Math.floor(Math.random() * topMoves.length)];

    return selectedMove;
}

function playAITurn() {
    if (gameOver || currentPlayer !== 2) {
        return;
    }

    if (movesLeft > 0) {
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
    }
}
