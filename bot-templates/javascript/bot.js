const Game = require('./game');
const WebSocket = require('ws');

// Configuration
const BACKEND_URL = process.env.BACKEND_URL || 'ws://localhost:8080/ws';

class Bot {
    constructor(url) {
        this.url = url;
        this.ws = null;
        this.game = null;
        this.userId = null;
        this.username = null;
        this.gameId = null;
        this.yourPlayer = null;
    }

    connect() {
        this.ws = new WebSocket(this.url);

        this.ws.on('open', () => {
            console.log(`Connected to ${this.url}`);
        });

        this.ws.on('message', (data) => {
            try {
                const msg = JSON.parse(data);
                this.handleMessage(msg);
            } catch (e) {
                console.error('Error parsing message:', e);
            }
        });

        this.ws.on('close', () => {
            console.log('Connection closed');
            // Optional: Reconnect logic here
        });

        this.ws.on('error', (err) => {
            console.error('Connection error:', err);
        });
    }

    handleMessage(msg) {
        switch (msg.type) {
            case 'welcome':
                this.userId = msg.userId;
                this.username = msg.username;
                console.log(`Bot registered as ${this.username}`);
                break;

            case 'bot_wanted':
                console.log(`Bot requested for lobby ${msg.lobbyId}`);
                this.joinLobby(msg.lobbyId);
                break;

            case 'lobby_joined':
                console.log('Joined lobby');
                break;

            case 'multiplayer_game_start':
                this.gameId = msg.gameId;
                this.yourPlayer = msg.yourPlayer;
                this.game = new Game(msg.rows, msg.cols, this.yourPlayer);
                console.log(`Game started, I am player ${this.yourPlayer}`);
                break;

            case 'turn_change':
                if (msg.player === this.yourPlayer) {
                    console.log(`My turn! Moves left: ${msg.movesLeft}`);
                    this.makeMoves(msg.movesLeft);
                }
                break;

            case 'move_made':
                if (msg.row !== undefined && msg.col !== undefined) {
                    this.game.applyMove(msg.row, msg.col, msg.player);
                }
                break;

            case 'game_end':
                console.log(`Game ended, winner: ${msg.winner}`);
                this.game = null;
                this.gameId = null;
                break;
        }
    }

    joinLobby(lobbyId) {
        this.send({
            type: 'join_lobby',
            lobbyId: lobbyId
        });
    }

    async makeMoves(movesLeft) {
        let currentMoves = movesLeft;
        while (currentMoves > 0) {
            const move = this.findMove();
            if (move) {
                console.log(`Sending move: ${move.row}, ${move.col}`);
                this.send({
                    type: 'move',
                    gameId: this.gameId,
                    row: move.row,
                    col: move.col
                });

                // Slight delay to prevent flooding
                await new Promise(resolve => setTimeout(resolve, 100));
                currentMoves--;
            } else {
                console.log('No valid moves found');
                break;
            }
        }
    }

    findMove() {
        // Simple random strategy
        const candidates = [];
        for (let r = 0; r < this.game.rows; r++) {
            for (let c = 0; c < this.game.cols; c++) {
                candidates.push({ row: r, col: c });
            }
        }

        // Shuffle candidates
        candidates.sort(() => Math.random() - 0.5);

        for (const pos of candidates) {
            if (this.game.isValidMove(pos.row, pos.col, this.yourPlayer)) {
                return pos;
            }
        }
        return null;
    }

    send(data) {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(JSON.stringify(data));
        }
    }
}

const bot = new Bot(BACKEND_URL);
bot.connect();
