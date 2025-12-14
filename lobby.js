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
        this.lobbiesList = document.getElementById('lobbies-list');

        // Current lobby controls
        this.lobbyDetails = document.getElementById('lobby-details');
        this.leaveLobbyBtn = document.getElementById('leave-lobby-button');
        this.addBotBtn = document.getElementById('add-bot-button');
        this.startGameBtn = document.getElementById('start-game-button');

        // Chat elements
        this.chatLog = document.getElementById('lobby-chat-log');
        this.chatButtons = document.querySelectorAll('.chat-btn');
        this.chatLastSent = 0; // Timestamp for rate limiting
        this.chatCount = 0;    // Counter for rate limiting window
        this.chatWindowStart = 0;
    }

    attachEventListeners() {
        // Mode toggle
        this.mode1v1Btn.addEventListener('click', () => this.switchMode('1v1'));
        this.modeMultiplayerBtn.addEventListener('click', () => this.switchMode('multiplayer'));

        // Lobby actions
        this.createLobbyBtn.addEventListener('click', () => this.createLobby());
        this.leaveLobbyBtn.addEventListener('click', () => this.leaveLobby());
        this.addBotBtn.addEventListener('click', () => this.addBot());
        this.startGameBtn.addEventListener('click', () => this.startGame());

        // Chat actions
        this.chatButtons.forEach(btn => {
            btn.addEventListener('click', () => {
                const msgId = btn.getAttribute('data-msg');
                this.sendQuickChat(msgId);
            });
        });
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

        this.mpClient.send({
            type: 'create_lobby',
            rows: rows,
            cols: cols,
            maxPlayers: 4  // Always create 4-slot lobbies
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
        // Get current AI coefficients from the global aiCoeffs object
        // (defined in ai.js and modified via UI controls)
        const botSettings = {
            materialWeight: aiCoeffs.materialWeight,
            mobilityWeight: aiCoeffs.mobilityWeight,
            positionWeight: aiCoeffs.positionWeight,
            redundancyWeight: aiCoeffs.redundancyWeight,
            cohesionWeight: aiCoeffs.cohesionWeight,
            searchDepth: 5  // Increased from 4 due to performance optimizations
        };

        this.mpClient.send({
            type: 'add_bot',
            botSettings: botSettings
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
        this.clearChatLog();
    }

    handleLobbyJoined(msg) {
        this.currentLobby = msg.lobby;
        this.isInLobby = true;
        this.isHost = false;  // Not the host, just a player
        this.showLobbyView();
        this.updateLobbyDisplay();
        this.clearChatLog();
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
        this.clearChatLog();
    }

    // Chat Functions

    sendQuickChat(messageId) {
        const now = Date.now();

        // Rate limiting: max 3 messages per 10 seconds
        if (now - this.chatWindowStart > 10000) {
            // Reset window
            this.chatWindowStart = now;
            this.chatCount = 0;
        }

        if (this.chatCount >= 3) {
            // Rate limited
            this.showChatRateLimit();
            return;
        }

        this.chatCount++;
        this.chatLastSent = now;

        this.mpClient.send({
            type: 'lobby_chat',
            messageId: messageId,
            // fallback content in English if translation fails on other end (or for future custom text)
            content: i18n.translations['en'][messageId] || messageId
        });

        // Add visual feedback (disable buttons briefly)
        this.disableChatButtons(1000);
    }

    handleLobbyChat(msg) {
        if (!this.isInLobby) return;

        const isSelf = msg.fromUserId === this.mpClient.userId;
        const senderName = isSelf ? 'You' : msg.username;

        // Try to translate messageId, fallback to content
        let text = '';
        if (msg.messageId && i18n.t(msg.messageId) !== msg.messageId) {
            text = i18n.t(msg.messageId);
        } else {
            text = msg.content || msg.messageId;
        }

        this.addChatMessage(senderName, text, isSelf);
    }

    addChatMessage(sender, text, isSelf) {
        if (!this.chatLog) return;

        const msgEl = document.createElement('div');
        msgEl.className = `chat-message ${isSelf ? 'self' : 'other'}`;

        const senderEl = document.createElement('div');
        senderEl.className = 'chat-sender';
        senderEl.textContent = sender;

        const contentEl = document.createElement('div');
        contentEl.className = 'chat-content';
        contentEl.textContent = text;

        if (!isSelf) msgEl.appendChild(senderEl); // Don't show name for self to save space
        msgEl.appendChild(contentEl);

        this.chatLog.appendChild(msgEl);
        this.chatLog.scrollTop = this.chatLog.scrollHeight;
    }

    clearChatLog() {
        if (this.chatLog) this.chatLog.innerHTML = '';
        this.chatCount = 0;
        this.chatWindowStart = 0;
    }

    disableChatButtons(duration) {
        this.chatButtons.forEach(btn => btn.disabled = true);
        setTimeout(() => {
            this.chatButtons.forEach(btn => btn.disabled = false);
        }, duration);
    }

    showChatRateLimit() {
        this.addChatMessage('System', i18n.t('chatRateLimit'), false);
        this.disableChatButtons(2000);
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
