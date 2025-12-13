let rows, cols, board, currentPlayer, movesLeft, player1Base, player2Base, gameOver, aiEnabled;
let gameBoard, statusDisplay, newGameButton, rowsInput, colsInput, aiEnabledCheckbox, putNeutralsButton, aiDepthInput, aiDepthSetting, aiTimeInput, aiTimeSetting, resignButton;
// Track neutral usage for all 4 players (index 0-3 for players 1-4)
let playerNeutralsUsed = [false, false, false, false];
let playerNeutralsStarted = [false, false, false, false];
let neutralMode = false;
let neutralsPlaced = 0;
// Legacy variables for backward compatibility
let player1NeutralsUsed = false;
let player2NeutralsUsed = false;
let player1NeutralsStarted = false;
let player2NeutralsStarted = false;
// Multiplayer mode variables
let playerBases = []; // Array of {row, col} for each player

function isConnectedToBase(startRow, startCol, player) {
    let base;
    if (typeof mpClient !== 'undefined' && mpClient.isMultiplayerGame) {
        // Multiplayer mode - use playerBases array
        if (playerBases[player - 1]) {
            base = playerBases[player - 1];
        } else {
            return false;
        }
    } else {
        // 1v1 mode - use player1Base or player2Base
        base = player === 1 ? player1Base : player2Base;
    }

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

    // Check if cell is empty or belongs to an opponent
    if (cellValue !== null) {
        const cellStr = String(cellValue);
        if (cellStr.startsWith(player.toString())) {
            return false; // Cannot place on own cell
        }
    }

    // Check if adjacent to own territory connected to base
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

const playerSymbols = ['X', 'O', '△', '□'];

function calculateCellSize() {
    // Get available width (viewport width minus some padding)
    const padding = 20; // 10px on each side
    const availableWidth = window.innerWidth - padding;

    // Calculate cell size based on columns
    // Default to 40px but shrink if needed to fit screen
    const maxCellSize = 40;
    const calculatedSize = Math.floor(availableWidth / cols);

    // Use the smaller of maxCellSize or calculated size, but at least 20px
    return Math.max(20, Math.min(maxCellSize, calculatedSize));
}

function renderBoard() {
    const cellSize = calculateCellSize();
    gameBoard.innerHTML = '';
    gameBoard.style.gridTemplateColumns = `repeat(${cols}, ${cellSize}px)`;
    gameBoard.style.gridTemplateRows = `repeat(${rows}, ${cellSize}px)`;

    // Update CSS custom property for cell size
    document.documentElement.style.setProperty('--cell-size', `${cellSize}px`);
    document.documentElement.style.setProperty('--cell-font-size', `${Math.max(12, Math.floor(cellSize * 0.6))}px`);
    for (let i = 0; i < rows; i++) {
        for (let j = 0; j < cols; j++) {
            const cell = document.createElement('div');
            cell.classList.add('cell');
            cell.dataset.row = i;
            cell.dataset.col = j;

            const cellValue = board[i][j];

            // Handle player cells (1, 2, 3, 4)
            for (let p = 1; p <= 4; p++) {
                if (cellValue === p) {
                    cell.classList.add(`player${p}`);
                    cell.textContent = playerSymbols[p - 1];
                } else if (cellValue === `${p}-fortified`) {
                    cell.classList.add(`player${p}-fortified`);
                    cell.textContent = playerSymbols[p - 1];
                } else if (cellValue === `${p}-base`) {
                    cell.classList.add(`player${p}-base`);
                    cell.textContent = playerSymbols[p - 1];
                }
            }

            if (cellValue === 'killed') {
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
        const playerSymbol = mpClient.playerSymbol || playerSymbols[mpClient.yourPlayer - 1];

        if (neutralMode) {
            statusDisplay.textContent = i18n.t('placeNeutral', { count: 2 - neutralsPlaced });
            if (statusDisplay) statusDisplay.classList.add('your-turn');
        } else if (isYourTurn) {
            if (mpClient.isMultiplayerGame) {
                // Multiplayer 3-4 players mode
                statusDisplay.textContent = `Your turn as ${playerSymbol}! (${movesLeft} moves left)`;
            } else {
                // 1v1 mode
                statusDisplay.textContent = i18n.t('yourTurn', { symbol: playerSymbol, opponent: mpClient.opponentUsername, moves: movesLeft });
            }
            if (statusDisplay) statusDisplay.classList.add('your-turn');
        } else {
            if (mpClient.isMultiplayerGame) {
                // Multiplayer 3-4 players mode
                const currentPlayerName = mpClient.getPlayerName(currentPlayer);
                statusDisplay.textContent = `${currentPlayerName}'s turn (${playerSymbols[currentPlayer - 1]})...`;
            } else {
                // 1v1 mode
                statusDisplay.textContent = i18n.t('opponentTurn', { opponent: mpClient.opponentUsername });
            }
            if (statusDisplay) statusDisplay.classList.remove('your-turn');
        }
        return;
    }

    // Local mode status - remove animation
    if (statusDisplay) statusDisplay.classList.remove('your-turn');

    if (neutralMode) {
        statusDisplay.textContent = i18n.t('placeNeutralPlayer', { player: currentPlayer, count: 2 - neutralsPlaced });
    } else {
        statusDisplay.textContent = i18n.t('playerTurn', { player: currentPlayer, moves: movesLeft });
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
        // Only show button for current player's turn
        // In multiplayer mode, only show for your turn; in local mode, show for both players (unless AI is playing)
        const isMultiplayer = typeof mpClient !== 'undefined' && mpClient.multiplayerMode;
        const yourPlayer = isMultiplayer ? mpClient.yourPlayer : null;

        // Reset button text if it's not the current player's turn (e.g., opponent's turn)
        if (isMultiplayer && currentPlayer !== yourPlayer && neutralMode) {
            neutralMode = false;
            neutralsPlaced = 0;
            if (window.neutralCells) window.neutralCells = [];
            putNeutralsButton.textContent = 'Place Neutrals';
        }

        // Determine if we should show the button
        let shouldShowButton = false;

        // Check if it's your turn
        let isYourTurn = false;
        if (isMultiplayer) {
            // Multiplayer: only show when it's your player's turn
            isYourTurn = (currentPlayer === yourPlayer);
        } else {
            // Local mode
            if (currentPlayer === 1) {
                isYourTurn = true; // Always show for player 1 in local mode
            } else if (currentPlayer === 2) {
                isYourTurn = !aiEnabled; // Only show for player 2 if not AI (hotseat mode)
            }
            // Players 3-4 not supported in local mode
        }

        // Check neutral usage for current player (support all 4 players)
        if (isYourTurn && currentPlayer >= 1 && currentPlayer <= 4) {
            const playerIndex = currentPlayer - 1;
            const neutralsUsed = playerNeutralsUsed[playerIndex];
            const neutralsStarted = playerNeutralsStarted[playerIndex];
            const playerCells = countNonFortifiedCells(currentPlayer);

            shouldShowButton = !neutralsUsed && !neutralsStarted && playerCells >= 2;
        }

        putNeutralsButton.style.display = shouldShowButton ? 'inline-block' : 'none';
    }

    // In local mode, check for no moves condition
    if (typeof mpClient === 'undefined' || !mpClient.multiplayerMode) {
        if (!gameOver && !canMakeMove(currentPlayer)) {
            const winner = currentPlayer === 1 ? 2 : 1;
            statusDisplay.textContent = i18n.t('noMoreMoves', { winner: winner, player: currentPlayer });
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
            statusDisplay.textContent = youWon ? i18n.t('youWin') : i18n.t('youLose');
            // Don't show rematch button in multiplayer (doesn't work)
        } else {
            // Local mode
            statusDisplay.textContent = i18n.t('playerWins', { player: winner });
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
            
            // Mark that player started using neutrals (hide button for rest of game)
            if (neutralsPlaced === 1 && currentPlayer >= 1 && currentPlayer <= 4) {
                const playerIndex = currentPlayer - 1;
                playerNeutralsStarted[playerIndex] = true;
                // Update legacy variables for backward compatibility
                if (currentPlayer === 1) player1NeutralsStarted = true;
                if (currentPlayer === 2) player2NeutralsStarted = true;

                if (putNeutralsButton) {
                    putNeutralsButton.style.display = 'none';
                }
            }

            if (neutralsPlaced === 2 && currentPlayer >= 1 && currentPlayer <= 4) {
                const playerIndex = currentPlayer - 1;
                playerNeutralsUsed[playerIndex] = true;
                // Update legacy variables for backward compatibility
                if (currentPlayer === 1) player1NeutralsUsed = true;
                if (currentPlayer === 2) player2NeutralsUsed = true;

                neutralMode = false;
                neutralsPlaced = 0;

                // Reset button text and hide button after use (one-time ability)
                if (putNeutralsButton) {
                    putNeutralsButton.textContent = 'Place Neutrals';
                    putNeutralsButton.style.display = 'none';
                }

                // Send neutrals to server in multiplayer
                if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
                    mpClient.sendNeutrals(window.neutralCells);
                    window.neutralCells = [];
                    // Don't call endTurn() in multiplayer - server handles it
                } else {
                    // Local mode only
                    endTurn();
                }
            }
        }
        return;
    }

    if (movesLeft > 0 && isValidMove(row, col, currentPlayer)) {
        // In multiplayer mode, send to server and wait for response
        if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
            mpClient.sendMove(row, col);
            // Don't apply locally - wait for server's move_made message
            // Optimistically decrement to prevent spam clicks
            movesLeft--;
            updateStatus();
            return;
        }

        // Local mode only - apply move locally
        const cellValue = board[row][col];

        if (cellValue === null) {
            // Place on empty cell - just the number, not fortified
            board[row][col] = currentPlayer;
        } else {
            // Attacking opponent's cell - it becomes ours and fortified
            // This should only happen if it's a non-fortified, non-base opponent cell
            const cellStr = String(cellValue);

            // Check it's opponent's non-fortified cell
            if (!cellStr.includes('fortified') && !cellStr.includes('base') &&
                !cellStr.startsWith(currentPlayer.toString())) {
                // Capture it and make it fortified
                board[row][col] = `${currentPlayer}-fortified`;
            } else {
                return; // Invalid attack
            }
        }

        // Local mode only - manage movesLeft locally
        movesLeft--;
        renderBoard();

        if (!canMakeMove(currentPlayer)) {
            const winner = currentPlayer === 1 ? 2 : 1;
            statusDisplay.textContent = i18n.t('noMoreMoves', { winner: winner, player: currentPlayer });
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

function handleResign() {
    if (gameOver) return;

    // Multiplayer mode - send resign to server
    if (typeof mpClient !== 'undefined' && mpClient.multiplayerMode) {
        mpClient.sendResign();
        return;
    }

    // Local mode - current player loses
    gameOver = true;
    const winner = currentPlayer === 1 ? 2 : 1;
    statusDisplay.textContent = `Player ${currentPlayer} resigned. Player ${winner} wins!`;
    if (resignButton) resignButton.style.display = 'none';
}

document.addEventListener('DOMContentLoaded', () => {
    gameBoard = document.getElementById('game-board');
    statusDisplay = document.getElementById('status');
    newGameButton = document.getElementById('new-game-button');
    rowsInput = document.getElementById('rows-input');
    colsInput = document.getElementById('cols-input');
    aiEnabledCheckbox = document.getElementById('ai-enabled');
    aiDepthInput = document.getElementById('ai-depth-input');
    aiDepthSetting = document.getElementById('ai-depth-setting');
    aiTimeInput = document.getElementById('ai-time-input');
    aiTimeSetting = document.getElementById('ai-time-setting');
    putNeutralsButton = document.getElementById('put-neutrals-button'); // May be null
    resignButton = document.getElementById('resign-button');

    // Show/hide AI depth setting and tuning based on AI checkbox
    aiEnabledCheckbox.addEventListener('change', () => {
        if (aiEnabledCheckbox.checked) {
            aiDepthSetting.style.display = 'block';
            aiTimeSetting.style.display = 'block';
            document.getElementById('ai-tuning-section').style.display = 'block';
        } else {
            aiDepthSetting.style.display = 'none';
            aiTimeSetting.style.display = 'none';
            document.getElementById('ai-tuning-section').style.display = 'none';
        }
    });

    // Update aiDepth variable when user changes the input
    aiDepthInput.addEventListener('change', () => {
        aiDepth = parseInt(aiDepthInput.value);
    });

    // Update aiTimeLimit variable when user changes the input
    aiTimeInput.addEventListener('change', () => {
        aiTimeLimit = parseInt(aiTimeInput.value);
    });

    // AI Tuning collapsible header
    document.getElementById('ai-tuning-header').addEventListener('click', () => {
        const controls = document.getElementById('ai-tuning-controls');
        const header = document.getElementById('ai-tuning-header');
        if (controls.style.display === 'none') {
            controls.style.display = 'block';
            header.innerHTML = '⚙️ AI Strategy Weights <span style="float: right;">▲</span>';
        } else {
            controls.style.display = 'none';
            header.innerHTML = '⚙️ AI Strategy Weights <span style="float: right;">▼</span>';
        }
    });

    // Wire up coefficient inputs (optimized to 5 parameters)
    document.getElementById('coeff-material').addEventListener('input', (e) => {
        aiCoeffs.materialWeight = parseFloat(e.target.value);
    });
    document.getElementById('coeff-mobility').addEventListener('input', (e) => {
        aiCoeffs.mobilityWeight = parseFloat(e.target.value);
    });
    document.getElementById('coeff-position').addEventListener('input', (e) => {
        aiCoeffs.positionWeight = parseFloat(e.target.value);
    });
    document.getElementById('coeff-redundancy').addEventListener('input', (e) => {
        aiCoeffs.redundancyWeight = parseFloat(e.target.value);
    });
    document.getElementById('coeff-cohesion').addEventListener('input', (e) => {
        aiCoeffs.cohesionWeight = parseFloat(e.target.value);
    });

    // Reset coefficients to defaults
    document.getElementById('reset-coeffs-button').addEventListener('click', () => {
        aiCoeffs.materialWeight = 100;
        aiCoeffs.mobilityWeight = 50;
        aiCoeffs.positionWeight = 30;
        aiCoeffs.redundancyWeight = 40;
        aiCoeffs.cohesionWeight = 25;

        document.getElementById('coeff-material').value = 100;
        document.getElementById('coeff-mobility').value = 50;
        document.getElementById('coeff-position').value = 30;
        document.getElementById('coeff-redundancy').value = 40;
        document.getElementById('coeff-cohesion').value = 25;
    });

    function initGame() {
        rows = parseInt(rowsInput.value);
        cols = parseInt(colsInput.value);
        aiEnabled = aiEnabledCheckbox.checked;

        // Update AI depth from input
        if (aiDepthInput) {
            aiDepth = parseInt(aiDepthInput.value);
        }

        board = Array(rows).fill(null).map(() => Array(cols).fill(null));

        currentPlayer = 1;
        movesLeft = 3;
        gameOver = false;
        // Reset neutral tracking for all players
        playerNeutralsUsed = [false, false, false, false];
        playerNeutralsStarted = [false, false, false, false];
        // Legacy variables
        player1NeutralsUsed = false;
        player2NeutralsUsed = false;
        player1NeutralsStarted = false;
        player2NeutralsStarted = false;
        neutralMode = false;
        neutralsPlaced = 0;

        player1Base = { row: 0, col: 0 };
        player2Base = { row: rows - 1, col: cols - 1 };

        board[player1Base.row][player1Base.col] = '1-base';
        board[player2Base.row][player2Base.col] = '2-base';

        renderBoard();
        updateStatus();

        // Show resign button for local games
        if (resignButton) {
            resignButton.style.display = 'inline-block';
        }
        
        // Show neutral button for local games (only if player has enough cells)
        if (putNeutralsButton) {
            putNeutralsButton.textContent = 'Place Neutrals';
            if (countNonFortifiedCells(1) >= 2) {
                putNeutralsButton.style.display = 'inline-block';
            } else {
                putNeutralsButton.style.display = 'none';
            }
        }
    }

    newGameButton.addEventListener('click', initGame);
    aiEnabledCheckbox.addEventListener('change', () => {
        aiEnabled = aiEnabledCheckbox.checked;
    });
    if (putNeutralsButton) {
        putNeutralsButton.addEventListener('click', () => {
            // If already in neutral mode, clicking again cancels it (only if no cells placed yet)
            if (neutralMode && neutralsPlaced === 0) {
                // Reset neutral placement state
                neutralMode = false;
                if (window.neutralCells) window.neutralCells = [];
                if (putNeutralsButton) {
                    putNeutralsButton.textContent = 'Place Neutrals';
                }
                updateStatus();
                return;
            }
            
            // Otherwise, start neutral placement if conditions are met (support all 4 players)
            if (currentPlayer >= 1 && currentPlayer <= 4) {
                const playerIndex = currentPlayer - 1;
                const neutralsUsed = playerNeutralsUsed[playerIndex];
                const playerCells = countNonFortifiedCells(currentPlayer);

                if (!neutralsUsed && playerCells >= 2) {
                    neutralMode = true;
                    if (putNeutralsButton) {
                        putNeutralsButton.textContent = 'Cancel Neutral Placement';
                    }
                    updateStatus();
                } else {
                    console.log('Neutral placement conditions not met. Player:', currentPlayer, 'Used:', neutralsUsed, 'Cells:', playerCells);
                }
            }
        });
    }
    gameBoard.addEventListener('click', handleCellClick);

    // Resign button handler
    if (resignButton) {
        resignButton.addEventListener('click', handleResign);
    }

    // Leave game button handler
    const leaveGameButton = document.getElementById('leave-game-button');
    if (leaveGameButton) {
        leaveGameButton.addEventListener('click', () => {
            if (mpClient && mpClient.isMultiplayerGame) {
                mpClient.leaveGame();
            }
        });
    }

    // Handle window resize to recalculate cell sizes
    let resizeTimeout;
    window.addEventListener('resize', () => {
        clearTimeout(resizeTimeout);
        resizeTimeout = setTimeout(() => {
            if (board && board.length > 0) {
                renderBoard();
            }
        }, 150); // Debounce resize events
    });

    // Initial game start
    initGame();
});