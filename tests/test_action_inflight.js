const assert = require('assert');
const fs = require('fs');
const path = require('path');

global.WebSocket = {OPEN: 1};
global.CellFlag = {NORMAL: 0x00, BASE: 0x10, FORTIFIED: 0x20, KILLED: 0x30};
global.EMPTY = 0;
global.createCell = (player, flag) => player | flag;
global.renderBoard = () => {};
global.board = [];
global.rows = 0;
global.cols = 0;
global.gameOver = false;
global.playerBases = [];
global.player1Base = {};
global.player2Base = {};
global.playerNeutralsUsed = [];
global.player1NeutralsUsed = false;
global.player2NeutralsUsed = false;
const boardElement = {
    locked: false,
    ariaDisabled: 'false',
    classList: {toggle: (_name, locked) => { boardElement.locked = locked; }},
    setAttribute: (_name, value) => { boardElement.ariaDisabled = value; },
};
const neutralButton = {disabled: false};
global.document = {getElementById: id => id === 'game-board' ? boardElement : id === 'put-neutrals-button' ? neutralButton : null};
const {MultiplayerClient} = require('../multiplayer.js');

const client = new MultiplayerClient('test-session');
client.showNotification = () => {};
client.resetMoveTimer = () => {};
client.notifyYourTurn = () => {};
global.currentPlayer = 1;
global.movesLeft = 3;
global.gameHistory = {isHistoryMode: () => false, push: () => {}};
global.updateStatus = () => {};
const sent = [];
client.gameId = 'game';
client.multiplayerMode = true;
client.ws = {readyState: WebSocket.OPEN, send: payload => sent.push(JSON.parse(payload))};

assert.strictEqual(client.sendMove(0, 1), true);
assert.strictEqual(client.sendMove(0, 1), false, 'delayed acknowledgement must lock rapid duplicate clicks');
assert.strictEqual(sent.length, 1);
assert.strictEqual(sent[0].requestId, 'test-session:1');
assert.strictEqual(boardElement.locked, true);
assert.strictEqual(boardElement.ariaDisabled, 'true');
assert.strictEqual(neutralButton.disabled, true);

client.handleActionAck({type: 'action_ack', requestId: 'different'});
assert.ok(client.inFlightAction, 'unrelated acknowledgement must not unlock input');
client.handleError({type: 'error', requestId: 'stale', username: 'stale'});
assert.ok(client.inFlightAction, 'stale error must not unlock current input');
assert.strictEqual(boardElement.locked, true);
client.handleActionAck({type: 'action_ack', requestId: 'test-session:1'});
assert.strictEqual(client.inFlightAction, null);
assert.strictEqual(boardElement.locked, false);
assert.strictEqual(neutralButton.disabled, false);

assert.strictEqual(client.sendNeutrals([{row: 0, col: 1}, {row: 1, col: 0}]), true);
assert.strictEqual(sent[1].requestId, 'test-session:2', 'request IDs must combine session identity with a monotonic counter');
client.handleError({type: 'error', requestId: 'test-session:2', username: 'rejected'});
assert.strictEqual(client.inFlightAction, null, 'authoritative error must unlock input');

client.sendMove(1, 1);
client.handleTurnChange({player: 2, movesLeft: 3});
assert.strictEqual(client.inFlightAction, null, 'turn resync must unlock input');

client.yourPlayer = 2;
client.sendMove(1, 1);
client.handleActionAck({requestId: 'test-session:4', snapshot: {
    rows: 2, cols: 2, currentPlayer: 2, movesLeft: 2, gameOver: false,
    board: [[{Owner: 1, Kind: 2}, {Owner: 0, Kind: 0}], [{Owner: 2, Kind: 1}, {Owner: 2, Kind: 2}]],
    bases: [{Row: 0, Col: 0}, {Row: 1, Col: 1}], neutralUsed: [false, false],
}});
assert.strictEqual(global.board[1][0], 2, 'acknowledgement snapshot must authoritatively resync board');

const reloadedClient = new MultiplayerClient('reloaded-session');
const reloadedSent = [];
reloadedClient.gameId = 'game';
reloadedClient.ws = {readyState: WebSocket.OPEN, send: payload => reloadedSent.push(JSON.parse(payload))};
assert.strictEqual(reloadedClient.sendMove(0, 1), true);
assert.strictEqual(reloadedSent[0].requestId, 'reloaded-session:1');
assert.notStrictEqual(reloadedSent[0].requestId, sent[0].requestId, 'reload/session prefix must prevent accepted-ID collisions');

const script = fs.readFileSync(path.join(__dirname, '..', 'script.js'), 'utf8');
assert.match(script, /mpClient\.updateActionInputLock\(\)/);
assert.match(script, /putNeutralsButton\.disabled = inputLocked/);
assert.match(script, /mpClient\.multiplayerMode && !mpClient\.canSendGameAction\(\)/);

console.log('multiplayer in-flight action checks passed');
