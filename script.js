document.addEventListener('DOMContentLoaded', () => {
    const gameBoard = document.getElementById('game-board');
    const statusDisplay = document.getElementById('status');
    const passButton = document.getElementById('pass-button');
    const newGameButton = document.getElementById('new-game-button');
    const rowsInput = document.getElementById('rows-input');
    const colsInput = document.getElementById('cols-input');

    let rows = 10;
    let cols = 10;
    let board = [];
    let currentPlayer = 1;
    let movesLeft = 3;

    function initGame() {
        rows = parseInt(rowsInput.value);
        cols = parseInt(colsInput.value);

        board = Array(rows).fill(null).map(() => Array(cols).fill(null));

        currentPlayer = 1;
        movesLeft = 3;

        // Initial setup
        board[0][0] = 1;
        board[0][1] = 1;
        board[1][0] = 1;

        board[rows - 1][cols - 1] = 2;
        board[rows - 1][cols - 2] = 2;
        board[rows - 2][cols - 1] = 2;

        renderBoard();
        updateStatus();
    }

    function renderBoard() {
        gameBoard.innerHTML = '';
        gameBoard.style.gridTemplateColumns = `repeat(${cols}, 40px)`;
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
                }

                gameBoard.appendChild(cell);
            }
        }
    }

    passButton.addEventListener('click', endTurn);
    newGameButton.addEventListener('click', initGame);
    gameBoard.addEventListener('click', handleCellClick);


    function checkWinCondition() {
        const player1Pieces = board.flat().filter(cell => String(cell).startsWith('1')).length;
        const player2Pieces = board.flat().filter(cell => String(cell).startsWith('2')).length;

        if (player1Pieces === 0) {
            statusDisplay.textContent = 'Player 2 wins!';
        } else if (player2Pieces === 0) {
            statusDisplay.textContent = 'Player 1 wins!';
        }
    }

    function endTurn() {
        currentPlayer = currentPlayer === 1 ? 2 : 1;
        movesLeft = 3;
        updateStatus();
        checkWinCondition();
    }

    function handleCellClick(event) {
        const cell = event.target.closest('.cell');
        if (!cell) return;

        const row = parseInt(cell.dataset.row);
        const col = parseInt(cell.dataset.col);
        const cellValue = board[row][col];

        if (typeof cellValue === 'string' && cellValue.includes('fortified')) {
            return; // Cannot attack fortified cells
        }

        if (movesLeft > 0) {
            const opponent = currentPlayer === 1 ? 2 : 1;

            if (cellValue === null && isAdjacent(row, col, currentPlayer)) {
                board[row][col] = currentPlayer;
                movesLeft--;
            } else if (String(cellValue).startsWith(opponent) && isAdjacent(row, col, currentPlayer)) {
                board[row][col] = `${currentPlayer}-fortified`;
                movesLeft--;
            }

            if (movesLeft === 0) {
                endTurn();
            }

            renderBoard();
            updateStatus();
        }
    }

    function isAdjacent(row, col, player) {
        for (let i = -1; i <= 1; i++) {
            for (let j = -1; j <= 1; j++) {
                if (i === 0 && j === 0) continue;
                const newRow = row + i;
                const newCol = col + j;

                if (newRow >= 0 && newRow < rows && newCol >= 0 && newCol < cols) {
                    const adjacentCellValue = board[newRow][newCol];
                    if (adjacentCellValue && String(adjacentCellValue).startsWith(player)) {
                        return true;
                    }
                }
            }
        }
        return false;
    }

    function updateStatus() {
        statusDisplay.textContent = `Player ${currentPlayer}'s turn. Moves left: ${movesLeft}.`;
    }

    // Initial game start
    initGame();
});