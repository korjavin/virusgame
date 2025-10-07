function getAIMove() {
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
    const attackMoves = possibleMoves.filter(move => board[move.row][move.col] === 1);
    if (attackMoves.length > 0) {
        // For now, pick a random attack
        return attackMoves[Math.floor(Math.random() * attackMoves.length)];
    }

    // If no attacks, pick a random growth move
    return possibleMoves[Math.floor(Math.random() * possibleMoves.length)];
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
            } else if (cellValue === 1) {
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
