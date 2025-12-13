// Debug script for neutral button issue
console.log('🔍 Debugging Neutral Button Issue');

// Mock the game state variables
let currentPlayer = 1;
let player1NeutralsUsed = false;
let player2NeutralsUsed = false;
let neutralMode = false;
let neutralsPlaced = 0;
let putNeutralsButton = { textContent: 'Place Neutrals', style: { display: 'inline-block' } };

// Mock board with sufficient cells
const mockBoard = [];
for (let i = 0; i < 10; i++) {
    const row = [];
    for (let j = 0; j < 10; j++) {
        if (i < 2 && j < 2) {
            row.push(1); // Player 1 cells
        } else {
            row.push(null);
        }
    }
    mockBoard.push(row);
}

// Mock countNonFortifiedCells function
function countNonFortifiedCells(player) {
    return mockBoard.flat().filter(cell => cell === player).length;
}

console.log('Initial state:');
console.log('Current player:', currentPlayer);
console.log('Player 1 neutrals used:', player1NeutralsUsed);
console.log('Player 1 cells:', countNonFortifiedCells(1));
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Button text:', putNeutralsButton.textContent);

// Test button click
console.log('\n🔘 Simulating button click...');

// If already in neutral mode, clicking again cancels it (only if no cells placed yet)
if (neutralMode && neutralsPlaced === 0) {
    console.log('Canceling neutral placement...');
    neutralMode = false;
    neutralsPlaced = 0;
    putNeutralsButton.textContent = 'Place Neutrals';
} else {
    // Otherwise, start neutral placement if conditions are met
    if (currentPlayer === 1 && !player1NeutralsUsed && countNonFortifiedCells(1) >= 2) {
        console.log('Starting neutral placement...');
        neutralMode = true;
        putNeutralsButton.textContent = 'Cancel Neutral Placement';
    } else if (currentPlayer === 2 && !player2NeutralsUsed && countNonFortifiedCells(2) >= 2) {
        console.log('Starting neutral placement for player 2...');
        neutralMode = true;
        putNeutralsButton.textContent = 'Cancel Neutral Placement';
    } else {
        console.log('❌ Conditions not met!');
        console.log('Player:', currentPlayer);
        console.log('Player 1 used:', player1NeutralsUsed);
        console.log('Player 2 used:', player2NeutralsUsed);
        console.log('Player 1 cells:', countNonFortifiedCells(1));
        console.log('Player 2 cells:', countNonFortifiedCells(2));
    }
}

console.log('\nAfter button click:');
console.log('Neutral mode:', neutralMode);
console.log('Button text:', putNeutralsButton.textContent);
console.log('Button should now say "Cancel Neutral Placement"');

// Test cell placement
console.log('\n📍 Testing cell placement...');
if (neutralMode) {
    const row = 0, col = 0;
    const cellValue = mockBoard[row][col];
    console.log('Cell value before:', cellValue);
    
    if (cellValue === currentPlayer) {
        mockBoard[row][col] = 'killed';
        neutralsPlaced++;
        console.log('Cell placed as neutral');
        
        if (neutralsPlaced === 1 && putNeutralsButton) {
            putNeutralsButton.textContent = 'Complete Neutral Placement';
            console.log('Button text updated to "Complete Neutral Placement"');
        }
        
        console.log('Cell value after:', mockBoard[row][col]);
        console.log('Neutrals placed:', neutralsPlaced);
    }
}

console.log('\nFinal state:');
console.log('Neutral mode:', neutralMode);
console.log('Neutrals placed:', neutralsPlaced);
console.log('Button text:', putNeutralsButton.textContent);