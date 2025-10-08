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

        const opponent = msg.player;
        const cellValue = board[msg.row][msg.col];

        if (cellValue === null) {
            board[msg.row][msg.col] = opponent;
        } else {
            board[msg.row][msg.col] = `${opponent}-fortified`;
        }

        renderBoard();
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
        currentPlayer = msg.player;
        movesLeft = 3;
        updateStatus();
    }

    handleGameEnd(msg) {
        gameOver = true;
        const winnerText = msg.winner === this.yourPlayer ? 'You win!' : 'You lose!';
        statusDisplay.textContent = `Game Over! ${winnerText}`;
        this.showRematchButton();
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
    }

    endMultiplayerGame() {
        this.multiplayerMode = false;
        this.gameId = null;
        this.yourPlayer = null;
        this.opponentId = null;
        this.opponentUsername = null;

        // Reset status
        if (statusDisplay) {
            statusDisplay.textContent = 'Multiplayer game ended. Start a new local game or challenge another player.';
        }
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
