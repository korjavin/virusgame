// Multiplayer WebSocket client for Virus Game
// Version 1.3 - Mobile-friendly UI, custom notifications, board size

class MultiplayerClient {
    constructor() {
        this.ws = null;
        this.userId = null;
        this.username = null;
        this.gameId = null;
        this.yourPlayer = null;
        this.opponentId = null;
        this.opponentUsername = null;
        this.onlineUsers = [];
        this.pendingChallenges = new Map();
        this.connected = false;
        this.multiplayerMode = false;
        // Multiplayer game mode
        this.isMultiplayerGame = false;
        this.gamePlayers = [];
        this.playerSymbol = null;
        // Move timer
        this.moveTimeLeft = 120;
        this.moveTimerInterval = null;
    }

    connect() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        this.ws = new WebSocket(wsUrl);

        this.ws.onopen = () => {
            console.log('Connected to multiplayer server');
            this.connected = true;
            this.updateConnectionStatus(true);
        };

        this.ws.onmessage = (event) => {
            try {
                // Handle multiple JSON messages separated by newlines
                const messages = event.data.trim().split('\n');
                messages.forEach(msgStr => {
                    if (msgStr.trim()) {
                        const msg = JSON.parse(msgStr);
                        this.handleMessage(msg);
                    }
                });
            } catch (error) {
                console.error('Error parsing message:', error, 'Data:', event.data);
            }
        };

        this.ws.onclose = () => {
            console.log('Disconnected from multiplayer server');
            this.connected = false;
            this.updateConnectionStatus(false);
            // Attempt to reconnect after 3 seconds
            setTimeout(() => this.connect(), 3000);
        };

        this.ws.onerror = (error) => {
            console.error('WebSocket error:', error);
        };
    }

    disconnect() {
        if (this.ws) {
            this.ws.close();
        }
    }

    send(message) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(message));
        }
    }

    handleMessage(msg) {
        console.log('Received message:', msg);

        switch (msg.type) {
            case 'welcome':
                this.handleWelcome(msg);
                break;
            case 'users_update':
                this.handleUsersUpdate(msg);
                break;
            case 'challenge_received':
                this.handleChallengeReceived(msg);
                break;
            case 'challenge_declined':
                this.handleChallengeDeclined(msg);
                break;
            case 'game_start':
                this.handleGameStart(msg);
                break;
            case 'move_made':
                this.handleMoveMade(msg);
                break;
            case 'neutrals_placed':
                this.handleNeutralsPlaced(msg);
                break;
            case 'turn_change':
                this.handleTurnChange(msg);
                break;
            case 'game_end':
                this.handleGameEnd(msg);
                break;
            case 'rematch_received':
                this.handleRematchReceived(msg);
                break;
            case 'opponent_disconnected':
                this.handleOpponentDisconnected(msg);
                break;
            case 'error':
                this.handleError(msg);
                break;
            // Lobby messages
            case 'lobby_created':
                if (lobbyManager) lobbyManager.handleLobbyCreated(msg);
                break;
            case 'lobby_joined':
                if (lobbyManager) lobbyManager.handleLobbyJoined(msg);
                break;
            case 'lobby_update':
                if (lobbyManager) lobbyManager.handleLobbyUpdate(msg);
                break;
            case 'lobbies_list':
                if (lobbyManager) lobbyManager.handleLobbiesList(msg);
                break;
            case 'lobby_closed':
                if (lobbyManager) lobbyManager.handleLobbyClosed(msg);
                break;
            case 'multiplayer_game_start':
                this.handleMultiplayerGameStart(msg);
                break;
            case 'player_eliminated':
                this.handlePlayerEliminated(msg);
                break;
            case 'game_end':
                this.handleGameEnd(msg);
                break;
            case 'bot_wanted':
                // Human clients ignore this message
                // Only bot clients will respond to this signal
                console.log('Bot wanted signal received (ignored by human client):', msg.lobbyId);
                break;
        }
    }

    handleGameEnd(msg) {
        gameOver = true;
        this.stopMoveTimer();

        // Hide resign button
        const resignBtn = document.getElementById('resign-button');
        if (resignBtn) resignBtn.style.display = 'none';

        if (this.isMultiplayerGame) {
            // Multiplayer mode (3-4 players) - show leave game button
            this.showLeaveGameButton();
        } else {
            // 1v1 mode - show result and rematch button, auto-cleanup
            const winnerText = msg.winner === this.yourPlayer ? 'You win!' : 'You lose!';
            if (statusDisplay) {
                statusDisplay.textContent = `Game Over! ${winnerText}`;
            }
            this.showRematchButton();
            // Auto-cleanup 1v1 game state after a short delay
            setTimeout(() => {
                this.endMultiplayerGame();
            }, 100);
        }
    }

    handleWelcome(msg) {
        this.userId = msg.userId;
        this.username = msg.username;
        console.log(`Welcome! You are ${this.username} (${this.userId})`);
        this.updateWelcomeMessage();
    }

    handleUsersUpdate(msg) {
        this.onlineUsers = msg.users.filter(u => u.userId !== this.userId);
        this.updateUsersList();
    }

    handleChallengeReceived(msg) {
        this.pendingChallenges.set(msg.challengeId, {
            fromUserId: msg.fromUserId,
            fromUsername: msg.fromUsername,
        });
        this.showChallengeNotification(msg);
    }

    handleChallengeDeclined(msg) {
        this.showNotification('Challenge Declined', 'Your challenge was declined');
    }

    handleGameStart(msg) {
        this.gameId = msg.gameId;
        this.yourPlayer = msg.yourPlayer;
        this.opponentId = msg.opponentId;
        this.opponentUsername = msg.opponentUsername;
        this.multiplayerMode = true;

        // Start new game in multiplayer mode
        this.startMultiplayerGame(msg.rows, msg.cols);
    }

    handleMoveMade(msg) {
        // Apply opponent's move to the board
        if (msg.row === undefined || msg.col === undefined) {
            console.error('Move message missing row or col:', msg);
            return;
        }

        console.log('Move made received:', msg, 'movesLeft before:', movesLeft);

        const opponent = msg.player;
        const cellValue = board[msg.row][msg.col];

        if (cellValue === null) {
            board[msg.row][msg.col] = opponent;
        } else {
            board[msg.row][msg.col] = `${opponent}-fortified`;
        }

        // Update movesLeft from server
        if (msg.movesLeft !== undefined) {
            movesLeft = msg.movesLeft;
            console.log('movesLeft updated from server to:', movesLeft);
        }

        renderBoard();
        updateStatus();
        checkWinCondition();
    }

    handleNeutralsPlaced(msg) {
        // Apply opponent's neutral placement
        for (const cell of msg.cells) {
            board[cell.row][cell.col] = 'killed';
        }
        renderBoard();
    }

    handleTurnChange(msg) {
        console.log('Turn change received:', msg);
        currentPlayer = msg.player;
        movesLeft = msg.movesLeft !== undefined ? msg.movesLeft : 3;
        updateStatus();

        // Update players display in multiplayer mode
        if (this.isMultiplayerGame) {
            this.updatePlayersDisplay();
        }

        // Notify player if it's their turn
        if (currentPlayer === this.yourPlayer) {
            this.notifyYourTurn();
        }

        // Reset move timer
        this.resetMoveTimer();
    }

    handleRematchReceived(msg) {
        // Rematch is now just a regular challenge, so this won't be called
        // But keep it for compatibility
        this.showNotification('Rematch Request', `${this.opponentUsername} wants a rematch!`);
    }

    handleOpponentDisconnected(msg) {
        this.showNotification('Opponent Disconnected', 'Your opponent has disconnected', {duration: 10000});
        this.endMultiplayerGame();
    }

    handleError(msg) {
        this.showNotification('Error', msg.username || 'An error occurred');
    }

    resetMoveTimer() {
        // Stop existing timer
        this.stopMoveTimer();

        // Reset time to 120 seconds
        this.moveTimeLeft = 120;

        // Only start timer if it's multiplayer mode and we're in a game
        if (this.isMultiplayerGame && !gameOver) {
            this.updateResignButtonText();
            this.moveTimerInterval = setInterval(() => {
                this.moveTimeLeft--;
                this.updateResignButtonText();

                if (this.moveTimeLeft <= 0) {
                    this.stopMoveTimer();
                }
            }, 1000);
        }
    }

    stopMoveTimer() {
        if (this.moveTimerInterval) {
            clearInterval(this.moveTimerInterval);
            this.moveTimerInterval = null;
        }
        this.moveTimeLeft = 120;
        this.updateResignButtonText();
    }

    updateResignButtonText() {
        const resignBtn = document.getElementById('resign-button');
        if (resignBtn && resignBtn.style.display !== 'none') {
            resignBtn.textContent = 'Resign';
        }

        // Update separate timer display
        let timerDisplay = document.getElementById('move-timer-display');
        if (!timerDisplay) {
            // Create timer display element
            timerDisplay = document.createElement('div');
            timerDisplay.id = 'move-timer-display';
            timerDisplay.className = 'move-timer-display';
            const gameControls = document.getElementById('game-controls');
            if (gameControls) {
                gameControls.appendChild(timerDisplay);
            }
        }

        if (this.isMultiplayerGame && !gameOver) {
            timerDisplay.textContent = `seconds before resign ${this.moveTimeLeft}`;
            timerDisplay.style.display = 'block';
        } else {
            timerDisplay.style.display = 'none';
        }
    }

    challengeUser(userId) {
        // Get current board size settings
        const rows = rowsInput ? parseInt(rowsInput.value) || 10 : 10;
        const cols = colsInput ? parseInt(colsInput.value) || 10 : 10;

        this.send({
            type: 'challenge',
            targetUserId: userId,
            rows: rows,
            cols: cols,
        });
    }

    acceptChallenge(challengeId) {
        this.send({
            type: 'accept_challenge',
            challengeId: challengeId,
        });
        this.pendingChallenges.delete(challengeId);
    }

    declineChallenge(challengeId) {
        this.send({
            type: 'decline_challenge',
            challengeId: challengeId,
        });
        this.pendingChallenges.delete(challengeId);
    }

    sendMove(row, col) {
        this.send({
            type: 'move',
            gameId: this.gameId,
            row: row,
            col: col,
        });
    }

    sendNeutrals(cells) {
        this.send({
            type: 'neutrals',
            gameId: this.gameId,
            cells: cells,
        });
    }

    sendResign() {
        this.send({
            type: 'resign',
            gameId: this.gameId,
        });
        // Update local state
        gameOver = true;
        if (statusDisplay) {
            statusDisplay.textContent = 'You resigned. Game over.';
        }
        // Hide resign button
        const resignBtn = document.getElementById('resign-button');
        if (resignBtn) resignBtn.style.display = 'none';
        // Show rematch button
        this.showRematchButton();
    }

    requestRematch() {
        if (!this.opponentId) {
            this.showNotification('Error', 'No opponent to rematch with');
            return;
        }
        // Simply send a new challenge to the same opponent
        this.challengeUser(this.opponentId);
        this.showNotification('Rematch', `Rematch request sent to ${this.opponentUsername}!`);
    }

    acceptRematch(gameId) {
        // Not used - rematch is just a new challenge
    }

    startMultiplayerGame(rows, cols) {
        // Initialize game with multiplayer settings
        if (rowsInput) rowsInput.value = rows;
        if (colsInput) colsInput.value = cols;
        if (aiEnabledCheckbox) aiEnabledCheckbox.checked = false;

        // Reset game state
        initGameMultiplayer(rows, cols);

        // Update status to show opponent
        const playerSymbol = this.yourPlayer === 1 ? 'X' : 'O';
        const turnText = currentPlayer === this.yourPlayer ? 'Your turn!' : "Opponent's turn...";
        if (statusDisplay) {
            statusDisplay.textContent = `Playing as ${playerSymbol} against ${this.opponentUsername}. ${turnText}`;
        }

        // Show resign button
        const resignBtn = document.getElementById('resign-button');
        if (resignBtn) resignBtn.style.display = 'inline-block';

        // Start move timer
        this.resetMoveTimer();
    }

    endMultiplayerGame() {
        // Stop move timer
        this.stopMoveTimer();
        this.multiplayerMode = false;
        this.isMultiplayerGame = false;
        this.gameId = null;
        this.yourPlayer = null;
        this.opponentId = null;
        this.opponentUsername = null;
        this.gamePlayers = [];
        this.playerSymbol = null;

        // Reset status
        if (statusDisplay) {
            statusDisplay.textContent = 'Multiplayer game ended. Start a new local game or challenge another player.';
        }
    }

    handleMultiplayerGameStart(msg) {
        this.gameId = msg.gameId;
        this.yourPlayer = msg.yourPlayer;
        this.playerSymbol = msg.playerSymbol;
        this.gamePlayers = msg.gamePlayers || [];
        this.isMultiplayerGame = true;
        this.multiplayerMode = true;

        // Exit lobby view when game starts to avoid confusion
        if (typeof lobbyManager !== 'undefined' && lobbyManager) {
            lobbyManager.exitLobbyView();
        }

        // Start multiplayer game with more than 2 players
        this.startMultiplayerGameMode(msg.rows, msg.cols, msg.gamePlayers);
    }

    handlePlayerEliminated(msg) {
        // Update player status when eliminated
        const player = this.gamePlayers.find(p => p.playerIndex === msg.eliminatedPlayer);
        if (player) {
            player.isActive = false;
        }
        this.updatePlayersDisplay();
        this.showNotification('Player Eliminated', `${player ? player.username : 'Player'} has been eliminated!`);

        // Show leave game button if this player was eliminated
        if (msg.eliminatedPlayer === this.yourPlayer) {
            this.showLeaveGameButton();
        }
    }

    showLeaveGameButton() {
        // Only show leave game button in multiplayer mode (3-4 players), not 1v1
        if (!this.isMultiplayerGame) {
            return;
        }

        const resignBtn = document.getElementById('resign-button');
        const leaveGameBtn = document.getElementById('leave-game-button');

        if (resignBtn) resignBtn.style.display = 'none';
        if (leaveGameBtn) leaveGameBtn.style.display = 'inline-block';
    }

    leaveGame() {
        // Send leave game message to server
        this.send({
            type: 'leave_game',
            gameId: this.gameId
        });

        // Reset local state
        this.gameId = null;
        this.yourPlayer = null;
        this.gamePlayers = [];
        this.isMultiplayerGame = false;
        this.stopMoveTimer();

        // Hide game buttons and players display
        const resignBtn = document.getElementById('resign-button');
        const leaveGameBtn = document.getElementById('leave-game-button');
        const playersInfo = document.getElementById('players-info');

        if (resignBtn) resignBtn.style.display = 'none';
        if (leaveGameBtn) leaveGameBtn.style.display = 'none';
        if (playersInfo) playersInfo.remove();

        // Show notification
        this.showNotification('Left Game', 'You have left the game');

        // Start a new local game
        if (typeof initGame === 'function') {
            initGame();
        }
    }

    startMultiplayerGameMode(rows, cols, gamePlayers) {
        // Initialize game with multiplayer settings
        if (rowsInput) rowsInput.value = rows;
        if (colsInput) colsInput.value = cols;
        if (aiEnabledCheckbox) aiEnabledCheckbox.checked = false;

        // Reset game state for multiplayer
        initGameMultiplayerMode(rows, cols, gamePlayers, this.yourPlayer);

        // Update status
        if (statusDisplay) {
            const turnText = currentPlayer === this.yourPlayer ? 'Your turn!' : `${this.getPlayerName(currentPlayer)}'s turn...`;
            statusDisplay.textContent = `Playing as ${this.playerSymbol}. ${turnText}`;
        }

        // Show players display
        this.updatePlayersDisplay();

        // Show resign button
        const resignBtn = document.getElementById('resign-button');
        if (resignBtn) resignBtn.style.display = 'inline-block';

        // Notify if it's player's turn at game start
        if (currentPlayer === this.yourPlayer) {
            this.notifyYourTurn();
        }

        // Start move timer
        this.resetMoveTimer();
    }

    updatePlayersDisplay() {
        let playersInfoContainer = document.getElementById('players-info');
        if (!playersInfoContainer) {
            playersInfoContainer = document.createElement('div');
            playersInfoContainer.id = 'players-info';
            playersInfoContainer.className = 'players-info';
            const gameContainer = document.getElementById('game-container');
            gameContainer.insertBefore(playersInfoContainer, document.getElementById('game-board'));
        }

        playersInfoContainer.innerHTML = '';

        this.gamePlayers.forEach(player => {
            const card = document.createElement('div');
            card.className = 'player-info-card';
            if (!player.isActive) {
                card.classList.add('eliminated');
            }
            if (player.playerIndex === currentPlayer) {
                card.classList.add('current-turn');
            }

            const badge = document.createElement('div');
            badge.className = `player-symbol-badge symbol-${player.symbol}`;
            badge.textContent = player.symbol;

            const name = document.createElement('span');
            name.textContent = player.username + (player.playerIndex === this.yourPlayer ? ' (You)' : '');

            card.appendChild(badge);
            card.appendChild(name);
            playersInfoContainer.appendChild(card);
        });
    }

    getPlayerName(playerIndex) {
        const player = this.gamePlayers.find(p => p.playerIndex === playerIndex);
        return player ? player.username : `Player ${playerIndex}`;
    }

    updateConnectionStatus(connected) {
        const statusEl = document.getElementById('connection-status');
        if (statusEl) {
            statusEl.textContent = connected ? 'Connected' : 'Disconnected';
            statusEl.className = connected ? 'connected' : 'disconnected';
        }
    }

    updateWelcomeMessage() {
        const welcomeEl = document.getElementById('welcome-message');
        if (welcomeEl) {
            welcomeEl.textContent = `Welcome, ${this.username}!`;
        }
    }

    updateUsersList() {
        const usersListEl = document.getElementById('users-list');
        if (!usersListEl) return;

        usersListEl.innerHTML = '';

        if (this.onlineUsers.length === 0) {
            usersListEl.innerHTML = '<div class="no-users">No other players online</div>';
            return;
        }

        this.onlineUsers.forEach(user => {
            const userEl = document.createElement('div');
            userEl.className = 'user-item' + (user.inGame ? ' in-game' : '');
            userEl.innerHTML = `
                <span class="username">${user.username}</span>
                ${!user.inGame ? `<button class="challenge-btn" data-user-id="${user.userId}">Challenge</button>` : '<span class="status-text">(In Game)</span>'}
            `;
            usersListEl.appendChild(userEl);
        });

        // Add click handlers to challenge buttons
        document.querySelectorAll('.challenge-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const userId = btn.getAttribute('data-user-id');
                const user = this.onlineUsers.find(u => u.userId === userId);
                this.challengeUser(userId);
                this.showNotification('Challenge Sent', `Challenge sent to ${user.username}!`);
            });
        });
    }

    showChallengeNotification(msg) {
        const notification = document.createElement('div');
        notification.className = 'challenge-notification';
        notification.innerHTML = `
            <p><strong>${msg.fromUsername}</strong> challenges you to a game!</p>
            <button class="accept-btn">Accept</button>
            <button class="decline-btn">Decline</button>
        `;

        const container = document.getElementById('notifications');
        if (container) {
            container.appendChild(notification);
        }

        notification.querySelector('.accept-btn').addEventListener('click', () => {
            this.acceptChallenge(msg.challengeId);
            notification.remove();
        });

        notification.querySelector('.decline-btn').addEventListener('click', () => {
            this.declineChallenge(msg.challengeId);
            notification.remove();
        });

        // Auto-decline after 30 seconds
        setTimeout(() => {
            if (notification.parentNode) {
                this.declineChallenge(msg.challengeId);
                notification.remove();
            }
        }, 30000);
    }

    showRematchButton() {
        // Don't show rematch button in multiplayer mode
        if (this.isMultiplayerGame) {
            return;
        }

        const rematchBtn = document.getElementById('rematch-button');
        if (rematchBtn) {
            rematchBtn.style.display = 'block';
        }
    }

    showNotification(title, message, options = {}) {
        const notification = document.createElement('div');
        notification.className = 'custom-notification';
        notification.innerHTML = `
            <div class="notification-title">${title}</div>
            <div class="notification-message">${message}</div>
            ${options.buttons ? `<div class="notification-buttons">${options.buttons}</div>` : ''}
        `;

        const container = document.getElementById('notifications');
        if (container) {
            container.appendChild(notification);
        }

        // Auto-remove after duration (default 5 seconds if no buttons/persistent flag)
        if (!options.buttons && !options.persistent) {
            const duration = options.duration || 5000;
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.remove();
                }
            }, duration);
        }

        return notification;
    }

    // Play a beep sound and vibrate to notify player it's their turn
    notifyYourTurn() {
        // Play beep sound using Web Audio API
        try {
            const audioContext = new (window.AudioContext || window.webkitAudioContext)();
            const oscillator = audioContext.createOscillator();
            const gainNode = audioContext.createGain();

            oscillator.connect(gainNode);
            gainNode.connect(audioContext.destination);

            // Two-tone beep for attention
            oscillator.frequency.setValueAtTime(880, audioContext.currentTime); // A5
            oscillator.frequency.setValueAtTime(1100, audioContext.currentTime + 0.1); // C#6

            gainNode.gain.setValueAtTime(0.3, audioContext.currentTime);
            gainNode.gain.exponentialRampToValueAtTime(0.01, audioContext.currentTime + 0.2);

            oscillator.start(audioContext.currentTime);
            oscillator.stop(audioContext.currentTime + 0.2);
        } catch (e) {
            console.log('Could not play turn notification sound:', e);
        }

        // Vibrate on mobile devices (if supported)
        if (navigator.vibrate) {
            navigator.vibrate([100, 50, 100]); // Two short vibrations
        }
    }
}

// Global multiplayer client instance
const mpClient = new MultiplayerClient();

// Initialize multiplayer game (called when game starts)
function initGameMultiplayer(rowsVal, colsVal) {
    rows = rowsVal;
    cols = colsVal;
    board = Array(rows).fill(null).map(() => Array(cols).fill(null));
    currentPlayer = 1;
    movesLeft = 3;
    gameOver = false;
    player1NeutralsUsed = false;
    player2NeutralsUsed = false;
    neutralMode = false;
    neutralsPlaced = 0;

    player1Base = { row: 0, col: 0 };
    player2Base = { row: rows - 1, col: cols - 1 };

    board[player1Base.row][player1Base.col] = '1-base';
    board[player2Base.row][player2Base.col] = '2-base';

    renderBoard();
    updateStatus();
}

// Initialize multiplayer game for 3-4 players
function initGameMultiplayerMode(rowsVal, colsVal, gamePlayers, yourPlayerIndex) {
    rows = rowsVal;
    cols = colsVal;
    board = Array(rows).fill(null).map(() => Array(cols).fill(null));
    currentPlayer = 1;
    movesLeft = 3;
    gameOver = false;
    neutralMode = false;
    neutralsPlaced = 0;

    // Base positions for 4 players
    const basePositions = [
        { row: 0, col: 0 },                // Player 1: top-left
        { row: rows - 1, col: cols - 1 },  // Player 2: bottom-right
        { row: 0, col: cols - 1 },         // Player 3: top-right
        { row: rows - 1, col: 0 }          // Player 4: bottom-left
    ];

    // Initialize playerBases array
    playerBases = basePositions;

    // Set bases for active players
    gamePlayers.forEach(player => {
        const playerIndex = player.playerIndex;
        const basePos = basePositions[playerIndex - 1];
        board[basePos.row][basePos.col] = `${playerIndex}-base`;
    });

    renderBoard();
    updateStatus();
}
