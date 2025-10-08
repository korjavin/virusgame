let rows, cols, board, currentPlayer, movesLeft, player1Base, player2Base, gameOver, aiEnabled;
let gameBoard, statusDisplay, newGameButton, rowsInput, colsInput, aiEnabledCheckbox, putNeutralsButton;
let player1NeutralsUsed = false;
let player2NeutralsUsed = false;
let neutralMode = false;
let neutralsPlaced = 0;

function isConnectedToBase(startRow, startCol, player) {
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
                    const cellValue = board[newRow][newCol];
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

function isValidMove(row, col, player) {
    const cellValue = board[row][col];
    if (typeof cellValue === 'string' && (cellValue.includes('fortified') || cellValue.includes('base'))) {
        return false; // Cannot attack fortified or base cells
    }

    const opponent = player === 1 ? 2 : 1;
    if (cellValue !== null && !String(cellValue).startsWith(opponent)) {
        return false; // Not an empty or opponent cell
    }

    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            const adjRow = row + i;
            const adjCol = col + j;

            if (adjRow >= 0 && adjRow < rows && adjCol >= 0 && adjCol < cols) {
                const adjCellValue = board[adjRow][adjCol];
                if (adjCellValue && String(adjCellValue).startsWith(player) && isConnectedToBase(adjRow, adjCol, player)) {
                    return true;
                }
            }
        }
    }
    return false;
}

function renderBoard() {
    gameBoard.innerHTML = '';
    gameBoard.style.gridTemplateColumns = `repeat(${cols}, 40px)`;
    gameBoard.style.gridTemplateRows = `repeat(${rows}, 40px)`;
    for (let i = 0; i < rows; i++) {
        for (let j = 0; j < cols; j++) {
            const cell = document.createElement('div');
            cell.classList.add('cell');
            cell.dataset.row = i;
            cell.dataset.col = j;

            const cellValue = board[i][j];
            if (cellValue === 1) {
                cell.classList.add('player1');
                cell.textContent = 'X';
            } else if (cellValue === 2) {
                cell.classList.add('player2');
                cell.textContent = 'O';
            } else if (cellValue === '1-fortified') {
                cell.classList.add('player1-fortified');
                cell.textContent = 'X';
            } else if (cellValue === '2-fortified') {
                cell.classList.add('player2-fortified');
                cell.textContent = 'O';
            } else if (cellValue === '1-base') {
                cell.classList.add('player1-base');
                cell.textContent = 'X';
            } else if (cellValue === '2-base') {
                cell.classList.add('player2-base');
                cell.textContent = 'O';
            } else if (cellValue === 'killed') {
                cell.classList.add('killed');
            }

            gameBoard.appendChild(cell);
        }
    }
}

function updateStatus() {
    if (gameOver) {
        // Remove animation when game is over
        if (statusDisplay) statusDisplay.classList.remove('your-turn');
        return;
    }

    // Multiplayer mode status
    if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
        const isYourTurn = currentPlayer === mpClient.yourPlayer;
        const playerSymbol = mpClient.yourPlayer === 1 ? 'X' : 'O';

        if (neutralMode) {
            statusDisplay.textContent = `Place ${2 - neutralsPlaced} neutral field(s).`;
            if (statusDisplay) statusDisplay.classList.add('your-turn');
        } else if (isYourTurn) {
            statusDisplay.textContent = `Your turn (${playerSymbol}) vs ${mpClient.opponentUsername}. Moves left: ${movesLeft}.`;
            if (statusDisplay) statusDisplay.classList.add('your-turn');
        } else {
            statusDisplay.textContent = `${mpClient.opponentUsername}'s turn. Waiting...`;
            if (statusDisplay) statusDisplay.classList.remove('your-turn');
        }
        return;
    }

    // Local mode status - remove animation
    if (statusDisplay) statusDisplay.classList.remove('your-turn');

    if (neutralMode) {
        statusDisplay.textContent = `Player ${currentPlayer}: Place ${2 - neutralsPlaced} neutral field(s).`;
    } else {
        statusDisplay.textContent = `Player ${currentPlayer}'s turn. Moves left: ${movesLeft}.`;
    }
}

function countNonFortifiedCells(player) {
    return board.flat().filter(cell => cell === player).length;
}

function endTurn() {
    currentPlayer = currentPlayer === 1 ? 2 : 1;
    movesLeft = 3;
    updateStatus();

    // In multiplayer mode, let the server check win conditions
    if (typeof mpClient === 'undefined' || !mpClient.multiplayerMode) {
        checkWinCondition();
    }

    if (putNeutralsButton) {
        if (currentPlayer === 1) {
            putNeutralsButton.disabled = player1NeutralsUsed || countNonFortifiedCells(1) < 2;
        } else {
            putNeutralsButton.disabled = player2NeutralsUsed || countNonFortifiedCells(2) < 2;
        }
    }

    // In local mode, check for no moves condition
    if (typeof mpClient === 'undefined' || !mpClient.multiplayerMode) {
        if (!gameOver && !canMakeMove(currentPlayer)) {
            const winner = currentPlayer === 1 ? 2 : 1;
            statusDisplay.textContent = `Player ${winner} wins! Player ${currentPlayer} has no more moves.`;
            gameOver = true;
        }
    }

    if (aiEnabled && currentPlayer === 2 && !gameOver) {
        setTimeout(playAITurn, 500);
    }
}

function checkWinCondition() {
    if (gameOver) return;
    const player1Pieces = board.flat().filter(cell => String(cell).startsWith('1')).length;
    const player2Pieces = board.flat().filter(cell => String(cell).startsWith('2')).length;

    let winner = 0;
    if (player1Pieces === 0) {
        winner = 2;
    } else if (player2Pieces === 0) {
        winner = 1;
    }

    if (winner > 0) {
        gameOver = true;

        // Multiplayer mode
        if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
            const youWon = winner === mpClient.yourPlayer;
            statusDisplay.textContent = youWon ? 'You win!' : 'You lose!';
            // Show rematch button
            const rematchBtn = document.getElementById('rematch-button');
            if (rematchBtn) rematchBtn.style.display = 'block';
        } else {
            // Local mode
            statusDisplay.textContent = `Player ${winner} wins!`;
        }
    }
}

function canMakeMove(player) {
    for (let r = 0; r < rows; r++) {
        for (let c = 0; c < cols; c++) {
            if (isValidMove(r, c, player)) {
                return true;
            }
        }
    }
    return false;
}

function handleCellClick(event) {
    if (gameOver || (aiEnabled && currentPlayer === 2)) return;

    // In multiplayer mode, check if it's your turn
    if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
        if (currentPlayer !== mpClient.yourPlayer) {
            return; // Not your turn
        }
    }

    const cell = event.target.closest('.cell');
    if (!cell) return;

    const row = parseInt(cell.dataset.row);
    const col = parseInt(cell.dataset.col);

    if (neutralMode) {
        const cellValue = board[row][col];
        if (cellValue === currentPlayer) {
            board[row][col] = 'killed';
            neutralsPlaced++;

            // Store cells for multiplayer
            if (!window.neutralCells) window.neutralCells = [];
            window.neutralCells.push({row, col});

            renderBoard();
            updateStatus();
            if (neutralsPlaced === 2) {
                if (currentPlayer === 1) {
                    player1NeutralsUsed = true;
                } else {
                    player2NeutralsUsed = true;
                }
                neutralMode = false;
                neutralsPlaced = 0;

                // Send neutrals to server in multiplayer
                if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
                    mpClient.sendNeutrals(window.neutralCells);
                    window.neutralCells = [];
                }

                endTurn();
            }
        }
        return;
    }

    if (movesLeft > 0 && isValidMove(row, col, currentPlayer)) {
        const opponent = currentPlayer === 1 ? 2 : 1;
        const cellValue = board[row][col];

        if (cellValue === null) {
            board[row][col] = currentPlayer;
        } else if (String(cellValue).startsWith(opponent)) {
            board[row][col] = `${currentPlayer}-fortified`;
        }

        movesLeft--;

        // Send move to server in multiplayer
        if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
            mpClient.sendMove(row, col);
        }

        renderBoard();

        if (!canMakeMove(currentPlayer)) {
            const winner = currentPlayer === 1 ? 2 : 1;
            statusDisplay.textContent = `Player ${winner} wins! Player ${currentPlayer} has no more moves.`;
            gameOver = true;
            return;
        }

        if (movesLeft === 0) {
            endTurn();
        } else {
            updateStatus();
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    gameBoard = document.getElementById('game-board');
    statusDisplay = document.getElementById('status');
    newGameButton = document.getElementById('new-game-button');
    rowsInput = document.getElementById('rows-input');
    colsInput = document.getElementById('cols-input');
    aiEnabledCheckbox = document.getElementById('ai-enabled');
    putNeutralsButton = document.getElementById('put-neutrals-button'); // May be null

    function initGame() {
        rows = parseInt(rowsInput.value);
        cols = parseInt(colsInput.value);
        aiEnabled = aiEnabledCheckbox.checked;

        board = Array(rows).fill(null).map(() => Array(cols).fill(null));

        currentPlayer = 1;
        movesLeft = 3;
        gameOver = false;
        player1NeutralsUsed = false;
        player2NeutralsUsed = false;
        neutralMode = false;
        neutralsPlaced = 0;
        if (putNeutralsButton) putNeutralsButton.disabled = false;

        player1Base = { row: 0, col: 0 };
        player2Base = { row: rows - 1, col: cols - 1 };

        board[player1Base.row][player1Base.col] = '1-base';
        board[player2Base.row][player2Base.col] = '2-base';

        renderBoard();
        updateStatus();

        if (putNeutralsButton && countNonFortifiedCells(1) < 2) {
            putNeutralsButton.disabled = true;
        }
    }

    newGameButton.addEventListener('click', initGame);
    aiEnabledCheckbox.addEventListener('change', () => {
        aiEnabled = aiEnabledCheckbox.checked;
    });
    if (putNeutralsButton) {
        putNeutralsButton.addEventListener('click', () => {
            if (currentPlayer === 1 && !player1NeutralsUsed && countNonFortifiedCells(1) >= 2) {
                neutralMode = true;
                updateStatus();
            } else if (currentPlayer === 2 && !player2NeutralsUsed && countNonFortifiedCells(2) >= 2) {
                neutralMode = true;
                updateStatus();
            }
        });
    }
    gameBoard.addEventListener('click', handleCellClick);

    // Initial game start
    initGame();
});