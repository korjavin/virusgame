// Test button visibility during game initialization
console.log('🧪 Testing Button Visibility During Game Initialization');

// Mock game state
let currentPlayer = 1;
let player1NeutralsUsed = false;
let putNeutralsButton = { 
    textContent: 'Place Neutrals', 
    style: { display: 'none' } // Initially hidden
};

// Mock board with sufficient cells for player 1
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

console.log('Initial button state:');
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Button text:', putNeutralsButton.textContent);

// Simulate game initialization
console.log('\n🎮 Simulating game initialization...');

// This is the logic from script.js that should show the button
if (putNeutralsButton) {
    if (countNonFortifiedCells(1) >= 2) {
        putNeutralsButton.style.display = 'inline-block';
        console.log('Button should be shown - player has enough cells');
    } else {
        putNeutralsButton.style.display = 'none';
        console.log('Button should be hidden - player does not have enough cells');
    }
}

console.log('\nAfter game initialization:');
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Button text:', putNeutralsButton.textContent);
console.log('Player 1 cells:', countNonFortifiedCells(1));

// Test button click
console.log('\n🔘 Testing button click...');
if (putNeutralsButton.style.display !== 'none') {
    console.log('Button is visible, click should work');
    
    // Simulate button click
    if (currentPlayer === 1 && !player1NeutralsUsed && countNonFortifiedCells(1) >= 2) {
        console.log('Conditions met, neutral mode should start');
        putNeutralsButton.textContent = 'Cancel Neutral Placement';
        console.log('Button text changed to:', putNeutralsButton.textContent);
    }
} else {
    console.log('Button is not visible, click will be ignored');
}

console.log('\n✅ Button visibility test completed');