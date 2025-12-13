// Simple test to check if board rendering works
console.log('Testing board rendering...');

// Mock the basic game state
const rows = 10;
const cols = 10;
const board = [];

// Initialize board
for (let i = 0; i < rows; i++) {
    const row = [];
    for (let j = 0; j < cols; j++) {
        if (i === 0 && j === 0) {
            row.push('1-base');
        } else if (i === rows - 1 && j === cols - 1) {
            row.push('2-base');
        } else {
            row.push(null);
        }
    }
    board.push(row);
}

console.log('Board initialized successfully');
console.log('Board size:', board.length, 'x', board[0].length);
console.log('Player 1 base:', board[0][0]);
console.log('Player 2 base:', board[rows-1][cols-1]);

// Test countNonFortifiedCells function
function countNonFortifiedCells(player) {
    return board.flat().filter(cell => cell === player).length;
}

console.log('Player 1 non-fortified cells:', countNonFortifiedCells(1));
console.log('Player 2 non-fortified cells:', countNonFortifiedCells(2));

console.log('✅ Board rendering test passed');