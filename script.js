const gameBoard = document.getElementById('game-board');
const statusDisplay = document.getElementById('status');

const boardSize = 10;
const board = Array(boardSize).fill(null).map(() => Array(boardSize).fill(null));

let currentPlayer = 1;
let movesLeft = 3;

function renderBoard() {
    gameBoard.innerHTML = '';
    for (let i = 0; i < boardSize; i++) {
        for (let j = 0; j < boardSize; j++) {
            const cell = document.createElement('div');
            cell.classList.add('cell');
            cell.dataset.row = i;
            cell.dataset.col = j;

            if (board[i][j] === 1) {
                cell.classList.add('player1');
                cell.textContent = 'X';
            } else if (board[i][j] === 2) {
                cell.classList.add('player2');
                cell.textContent = 'O';
            } else if (board[i][j] === 'killed') {
                cell.classList.add('killed');
            }

            cell.addEventListener('click', handleCellClick);
            gameBoard.appendChild(cell);
        }
    }
}

let gameMode = 'place'; // 'place' or 'kill'

const modeButton = document.getElementById('mode-button');

modeButton.addEventListener('click', () => {
    if (gameMode === 'place') {
        gameMode = 'kill';
        modeButton.textContent = 'Switch to Place Mode';
    } else {
        gameMode = 'place';
        modeButton.textContent = 'Switch to Kill Mode';
    }
    updateStatus();
});

const passButton = document.getElementById('pass-button');

passButton.addEventListener('click', endTurn);

function checkWinCondition() {
    const player1Pieces = board.flat().filter(cell => cell === 1).length;
    const player2Pieces = board.flat().filter(cell => cell === 2).length;

    if (player1Pieces === 0) {
        statusDisplay.textContent = 'Player 2 wins!';
        gameBoard.removeEventListener('click', handleCellClick);
    } else if (player2Pieces === 0) {
        statusDisplay.textContent = 'Player 1 wins!';
        gameBoard.removeEventListener('click', handleCellClick);
    }
}

function endTurn() {
    currentPlayer = currentPlayer === 1 ? 2 : 1;
    movesLeft = 3;
    gameMode = 'place';
    modeButton.textContent = 'Switch to Kill Mode';
    updateStatus();
    checkWinCondition();
}


const neutralButton = document.getElementById('neutral-button');

neutralButton.addEventListener('click', () => {
    gameMode = 'neutral';
    modeButton.textContent = 'Switch to Place Mode';
    updateStatus();
});

function handleCellClick(event) {
    const row = parseInt(event.target.dataset.row);
    const col = parseInt(event.target.dataset.col);

    if (movesLeft > 0) {
        if (gameMode === 'place') {
            if (board[row][col] === null && isAdjacent(row, col, currentPlayer)) {
                board[row][col] = currentPlayer;
                movesLeft--;
            }
        } else if (gameMode === 'kill') {
            const opponent = currentPlayer === 1 ? 2 : 1;
            if (board[row][col] === opponent && isAdjacent(row, col, currentPlayer)) {
                board[row][col] = 'killed';
                movesLeft--;
            }
        } else if (gameMode === 'neutral') {
            if (board[row][col] === null) {
                board[row][col] = 'killed';
                movesLeft--;
            }
        }

        if (movesLeft === 0) {
            endTurn();
        }

        renderBoard();
        updateStatus();
    }
}

function isAdjacent(row, col, player) {
    const opponent = player === 1 ? 2 : 1;
    const visited = new Set();

    function search(r, c) {
        if (r < 0 || r >= boardSize || c < 0 || c >= boardSize) {
            return false;
        }

        const key = `${r},${c}`;
        if (visited.has(key)) {
            return false;
        }
        visited.add(key);

        if (board[r][c] === player) {
            return true;
        }

        if (board[r][c] === opponent || board[r][c] === 'killed') {
            for (let i = -1; i <= 1; i++) {
                for (let j = -1; j <= 1; j++) {
                    if (i === 0 && j === 0) continue;
                    if (search(r + i, c + j)) {
                        return true;
                    }
                }
            }
        }

        return false;
    }

    for (let i = -1; i <= 1; i++) {
        for (let j = -1; j <= 1; j++) {
            if (i === 0 && j === 0) continue;
            if (search(row + i, col + j)) {
                return true;
            }
        }
    }

    return false;
}

function updateStatus() {
    statusDisplay.textContent = `Player ${currentPlayer}'s turn. Moves left: ${movesLeft}. Mode: ${gameMode}`;
}

// Initial setup
board[0][0] = 1;
board[0][1] = 1;
board[1][0] = 1;

board[boardSize - 1][boardSize - 1] = 2;
board[boardSize - 1][boardSize - 2] = 2;
board[boardSize - 2][boardSize - 1] = 2;

renderBoard();
updateStatus();
