// Lobby management for multiplayer mode
// Version 1.0

class LobbyManager {
    constructor(mpClient) {
        this.mpClient = mpClient;
        this.currentLobby = null;
        this.isInLobby = false;
        this.isHost = false;
        this.availableLobbies = [];

        this.initializeElements();
        this.attachEventListeners();
    }

    initializeElements() {
        // Mode buttons
        this.mode1v1Btn = document.getElementById('mode-1v1');
        this.modeMultiplayerBtn = document.getElementById('mode-multiplayer');

        // Sections
        this.usersSection = document.querySelector('.users-section');
        this.multiplayerSection = document.getElementById('multiplayer-section');
        this.currentLobbySection = document.getElementById('current-lobby-section');
        this.aiEnabledGroup = document.getElementById('ai-enabled-group');

        // Lobby controls
        this.createLobbyBtn = document.getElementById('create-lobby-button');
        this.refreshLobbiesBtn = document.getElementById('refresh-lobbies-button');
        this.lobbiesList = document.getElementById('lobbies-list');
        this.maxPlayersInput = document.getElementById('max-players-input');

        // Current lobby controls
        this.lobbyDetails = document.getElementById('lobby-details');
        this.leaveLobbyBtn = document.getElementById('leave-lobby-button');
        this.addBotBtn = document.getElementById('add-bot-button');
        this.startGameBtn = document.getElementById('start-game-button');
    }

    attachEventListeners() {
        // Mode toggle
        this.mode1v1Btn.addEventListener('click', () => this.switchMode('1v1'));
        this.modeMultiplayerBtn.addEventListener('click', () => this.switchMode('multiplayer'));

        // Lobby actions
        this.createLobbyBtn.addEventListener('click', () => this.createLobby());
        this.refreshLobbiesBtn.addEventListener('click', () => this.refreshLobbies());
        this.leaveLobbyBtn.addEventListener('click', () => this.leaveLobby());
        this.addBotBtn.addEventListener('click', () => this.addBot());
        this.startGameBtn.addEventListener('click', () => this.startGame());
    }

    switchMode(mode) {
        if (mode === '1v1') {
            this.mode1v1Btn.classList.add('active');
            this.modeMultiplayerBtn.classList.remove('active');

            // Show 1v1 elements
            this.usersSection.style.display = 'block';
            this.aiEnabledGroup.style.display = 'block';

            // Hide multiplayer elements
            this.multiplayerSection.style.display = 'none';
            this.currentLobbySection.style.display = 'none';
        } else {
            this.mode1v1Btn.classList.remove('active');
            this.modeMultiplayerBtn.classList.add('active');

            // Hide 1v1 elements
            this.usersSection.style.display = 'none';
            this.aiEnabledGroup.style.display = 'none';

            // Show multiplayer elements
            if (this.isInLobby) {
                this.multiplayerSection.style.display = 'none';
                this.currentLobbySection.style.display = 'block';
            } else {
                this.multiplayerSection.style.display = 'block';
                this.currentLobbySection.style.display = 'none';
                this.refreshLobbies();
            }
        }
    }

    createLobby() {
        const rows = parseInt(document.getElementById('rows-input').value) || 10;
        const cols = parseInt(document.getElementById('cols-input').value) || 10;
        const maxPlayers = parseInt(this.maxPlayersInput.value) || 4;

        this.mpClient.send({
            type: 'create_lobby',
            rows: rows,
            cols: cols,
            maxPlayers: maxPlayers
        });
    }

    refreshLobbies() {
        this.mpClient.send({
            type: 'get_lobbies'
        });
    }

    joinLobby(lobbyId) {
        this.mpClient.send({
            type: 'join_lobby',
            lobbyId: lobbyId
        });
    }

    leaveLobby() {
        this.mpClient.send({
            type: 'leave_lobby'
        });
        this.exitLobbyView();
    }

    addBot() {
        this.mpClient.send({
            type: 'add_bot'
        });
    }

    removeBot(slotIndex) {
        this.mpClient.send({
            type: 'remove_bot',
            slotIndex: slotIndex
        });
    }

    startGame() {
        this.mpClient.send({
            type: 'start_multiplayer_game'
        });
    }

    // Handle messages from server
    handleLobbyCreated(msg) {
        this.currentLobby = msg.lobby;
        this.isInLobby = true;
        this.isHost = true;
        this.showLobbyView();
        this.updateLobbyDisplay();
    }

    handleLobbyUpdate(msg) {
        this.currentLobby = msg.lobby;
        this.updateLobbyDisplay();
    }

    handleLobbiesList(msg) {
        this.availableLobbies = msg.lobbies || [];
        this.updateLobbiesList();
    }

    handleLobbyClosed(msg) {
        this.mpClient.showNotification('Lobby Closed', msg.username || 'The lobby was closed');
        this.exitLobbyView();
    }

    showLobbyView() {
        this.multiplayerSection.style.display = 'none';
        this.currentLobbySection.style.display = 'block';
    }

    exitLobbyView() {
        this.isInLobby = false;
        this.isHost = false;
        this.currentLobby = null;
        this.currentLobbySection.style.display = 'none';
        this.multiplayerSection.style.display = 'block';
        this.refreshLobbies();
    }

    updateLobbiesList() {
        if (this.availableLobbies.length === 0) {
            this.lobbiesList.innerHTML = '<div class="no-lobbies">No lobbies available</div>';
            return;
        }

        this.lobbiesList.innerHTML = '';
        this.availableLobbies.forEach(lobby => {
            const lobbyEl = document.createElement('div');
            lobbyEl.className = 'lobby-item';

            const playerCount = lobby.players.filter(p => !p.isEmpty).length;

            const playersDisplay = document.createElement('div');
            playersDisplay.className = 'lobby-players-display';
            lobby.players.forEach(player => {
                const badge = document.createElement('div');
                badge.className = 'lobby-player-badge';
                badge.textContent = player.symbol;

                if (player.isEmpty) {
                    badge.classList.add('empty');
                } else if (player.isBot) {
                    badge.classList.add('bot');
                } else {
                    badge.classList.add('filled');
                }

                playersDisplay.appendChild(badge);
            });

            lobbyEl.innerHTML = `
                <div class="lobby-item-header">${lobby.hostName}'s Lobby</div>
                <div class="lobby-item-info">${playerCount}/${lobby.maxPlayers} players</div>
            `;
            lobbyEl.appendChild(playersDisplay);

            lobbyEl.addEventListener('click', () => {
                this.joinLobby(lobby.lobbyId);
            });

            this.lobbiesList.appendChild(lobbyEl);
        });
    }

    updateLobbyDisplay() {
        if (!this.currentLobby) return;

        this.lobbyDetails.innerHTML = '';

        this.currentLobby.players.forEach((player, index) => {
            const slotEl = document.createElement('div');
            slotEl.className = 'lobby-slot';

            if (player.isEmpty) {
                slotEl.classList.add('empty');
                slotEl.innerHTML = `
                    <div class="lobby-slot-info">
                        <span class="lobby-slot-symbol">${player.symbol}</span>
                        <span>Empty Slot</span>
                    </div>
                `;
            } else if (player.isBot) {
                slotEl.classList.add('bot');
                slotEl.innerHTML = `
                    <div class="lobby-slot-info">
                        <span class="lobby-slot-symbol">${player.symbol}</span>
                        <span>${player.username}</span>
                    </div>
                `;

                if (this.isHost) {
                    const removeBtn = document.createElement('button');
                    removeBtn.textContent = 'Remove';
                    removeBtn.className = 'remove-bot-btn';
                    removeBtn.addEventListener('click', () => this.removeBot(index));
                    slotEl.appendChild(removeBtn);
                }
            } else {
                slotEl.classList.add('filled');
                slotEl.innerHTML = `
                    <div class="lobby-slot-info">
                        <span class="lobby-slot-symbol">${player.symbol}</span>
                        <span>${player.username}${player.username === this.mpClient.username ? ' (You)' : ''}</span>
                    </div>
                `;
            }

            this.lobbyDetails.appendChild(slotEl);
        });

        // Update host-only controls
        if (this.isHost) {
            this.addBotBtn.style.display = 'block';
            this.startGameBtn.style.display = 'block';

            // Enable start button if at least 2 players
            const playerCount = this.currentLobby.players.filter(p => !p.isEmpty).length;
            this.startGameBtn.disabled = playerCount < 2;
        } else {
            this.addBotBtn.style.display = 'none';
            this.startGameBtn.style.display = 'none';
        }
    }
}

// Will be initialized after mpClient is created
let lobbyManager = null;
