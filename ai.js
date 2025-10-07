function getAIMove() {
    const neutralMove = getAINeutralMove();
    if (neutralMove) {
        return neutralMove;
    }

    const possibleMoves = [];
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            if (isValidMove(r, c, 2)) {
                possibleMoves.push({ row: r, col: c });
            }
        }
    }

    if (possibleMoves.length === 0) {
        return null;
    }

    // Prioritize attacks
    const attackMoves = possibleMoves.filter(move => String(board[move.row][move.col]).startsWith('1'));
    if (attackMoves.length > 0) {
        // For now, pick a random attack
        return attackMoves[Math.floor(Math.random() * attackMoves.length)];
    }

    // If no attacks, pick a random growth move
    return possibleMoves[Math.floor(Math.random() * possibleMoves.length)];
}

function getAINeutralMove() {
    const possibleSacrifices = [];
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            if (board[r][c] === 2) { // AI is player 2
                possibleSacrifices.push({ row: r, col: c });
            }
        }
    }

    if (possibleSacrifices.length < 2) {
        return null; // Not enough pieces to sacrifice
    }

    const sacrificeCombinations = [];
    for (let i = 0; i < possibleSacrifices.length; i++) {
        for (let j = i + 1; j < possibleSacrifices.length; j++) {
            sacrificeCombinations.push([possibleSacrifices[i], possibleSacrifices[j]]);
        }
    }

    if (sacrificeCombinations.length === 0) {
        return null;
    }

    const bestMove = {
        sacrifices: [],
        neutrals: [],
        disconnected: 0
    };

    for (const sacrifices of sacrificeCombinations) {
        const adjacentCells = [];
        for (const sacrifice of sacrifices) {
            for (let i = -1; i <= 1; i++) {
                for (let j = -1; j <= 1; j++) {
                    if (i === 0 && j === 0) continue;
                    const newRow = sacrifice.row + i;
                    const newCol = sacrifice.col + j;

                    if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols && board[newRow][newCol] === null) {
                        adjacentCells.push({ row: newRow, col: newCol });
                    }
                }
            }
        }

        if (adjacentCells.length < 2) {
            continue;
        }

        const neutralCombinations = [];
        for (let i = 0; i < adjacentCells.length; i++) {
            for (let j = i + 1; j < adjacentCells.length; j++) {
                neutralCombinations.push([adjacentCells[i], adjacentCells[j]]);
            }
        }

        for (const neutrals of neutralCombinations) {
            const newBoard = JSON.parse(JSON.stringify(board));
            newBoard[sacrifices[0].row][sacrifices[0].col] = null;
            newBoard[sacrifices[1].row][sacrifices[1].col] = null;
            newBoard[neutrals[0].row][neutrals[0].col] = 'killed';
            newBoard[neutrals[1].row][neutrals[1].col] = 'killed';

            const disconnected = countDisconnectedOpponentCells(newBoard);

            if (disconnected > bestMove.disconnected) {
                bestMove.disconnected = disconnected;
                bestMove.sacrifices = sacrifices;
                bestMove.neutrals = neutrals;
            }
        }
    }

    if (bestMove.disconnected > 0) {
        return bestMove;
    }

    return null;
}

function countDisconnectedOpponentCells(currentBoard) {
    let disconnected = 0;
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            if (String(currentBoard[r][c]).startsWith('1') && !isConnectedToBaseWithBoard(r, c, 1, currentBoard)) {
                disconnected++;
            }
        }
    }
    return disconnected;
}

function isConnectedToBaseWithBoard(startRow, startCol, player, currentBoard) {
    const base = player === 1 ? player1Base : player2Base;
    const visited = new Set();
    const stack = [{ row: startRow, col: startCol }];
    visited.add(`${startRow},${startCol}`);

    while (stack.length > 0) {
        const { row, col } = stack.pop();

        if (row === base.row && col === base.col) {
            return true;
        }

        for (let i = -1; i <= 1; i++) {
            for (let j = -1; j <= 1; j++) {
                if (i === 0 && j === 0) continue;
                const newRow = row + i;
                const newCol = col + j;

                if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols && !visited.has(`${newRow},${newCol}`)) {
                    const cellValue = currentBoard[newRow][newCol];
                    if (cellValue && String(cellValue).startsWith(player)) {
                        visited.add(`${newRow},${newCol}`);
                        stack.push({ row: newRow, col: newCol });
                    }
                }
            }
        }
    }
    return false;
}

function playAITurn() {
    if (gameOver || currentPlayer !== 2) {
        return;
    }

    if (movesLeft > 0) {
        const move = getAIMove();
        if (move) {
            if (move.sacrifices) { // It's a neutral move
                board[move.sacrifices[0].row][move.sacrifices[0].col] = null;
                board[move.sacrifices[1].row][move.sacrifices[1].col] = null;
                board[move.neutrals[0].row][move.neutrals[0].col] = 'killed';
                board[move.neutrals[1].row][move.neutrals[1].col] = 'killed';
                player2NeutralsUsed = true;
                endTurn();
            } else { // It's a regular move
                const cellValue = board[move.row][move.col];
                if (cellValue === null) {
                    board[move.row][move.col] = 2;
                } else if (String(cellValue).startsWith('1')) {
                    board[move.row][move.col] = '2-fortified';
                }
                movesLeft--;
                renderBoard();
                updateStatus();

                if (movesLeft > 0) {
                    setTimeout(playAITurn, 500); // Make the next move after a short delay
                } else {
                    endTurn();
                }
            }
        } else {
            endTurn();
        }
    }
}
