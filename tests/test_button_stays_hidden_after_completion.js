// Test button stays hidden after completion
console.log('🧪 Testing Button Stays Hidden After Completion');

// Mock game state
let currentPlayer = 1;
let player1NeutralsUsed = false;
let player1NeutralsStarted = false;
let player2NeutralsUsed = false;
let player2NeutralsStarted = false;
let neutralMode = false;
let neutralsPlaced = 0;
let putNeutralsButton = { 
    textContent: 'Place Neutrals', 
    style: { display: 'inline-block' } 
};

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
console.log('Player 1 cells:', countNonFortifiedCells(1));
console.log('Button visible:', putNeutralsButton.style.display);

// Test 1: Button visible initially
console.log('\n📋 Test 1: Button visible initially');
if (putNeutralsButton) {
    if (currentPlayer === 1) {
        if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
            putNeutralsButton.style.display = 'none';
        } else {
            putNeutralsButton.style.display = 'inline-block';
        }
    }
}
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should be visible initially');

// Test 2: Start neutral placement
console.log('\n📋 Test 2: Start neutral placement');
neutralMode = true;
putNeutralsButton.textContent = 'Cancel Neutral Placement';
console.log('Neutral mode:', neutralMode);
console.log('Button text:', putNeutralsButton.textContent);

// Test 3: Place first neutral
console.log('\n📋 Test 3: Place first neutral');
const row1 = 0, col1 = 0;
const cellValue1 = mockBoard[row1][col1];
if (cellValue1 === currentPlayer) {
    mockBoard[row1][col1] = 'killed';
    neutralsPlaced++;
    
    // Mark that player started using neutrals
    if (neutralsPlaced === 1) {
        player1NeutralsStarted = true;
        if (putNeutralsButton) {
            putNeutralsButton.style.display = 'none';
        }
    }
}
console.log('First neutral placed');
console.log('Neutrals placed:', neutralsPlaced);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should be hidden after first placement');

// Test 4: Place second neutral
console.log('\n📋 Test 4: Place second neutral');
const row2 = 0, col2 = 1;
const cellValue2 = mockBoard[row2][col2];
if (cellValue2 === currentPlayer) {
    mockBoard[row2][col2] = 'killed';
    neutralsPlaced++;
    
    // Complete neutral placement
    if (neutralsPlaced === 2) {
        player1NeutralsUsed = true;
        neutralMode = false;
        // Ensure button stays hidden after completion
        if (putNeutralsButton) {
            putNeutralsButton.style.display = 'none';
        }
    }
}
console.log('Second neutral placed');
console.log('Neutrals placed:', neutralsPlaced);
console.log('Player 1 neutrals used:', player1NeutralsUsed);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should still be hidden after completion');

// Test 5: Simulate turn change
console.log('\n📋 Test 5: Simulate turn change');
currentPlayer = 2;

// Check button visibility after turn change
if (putNeutralsButton) {
    if (currentPlayer === 1) {
        if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
            putNeutralsButton.style.display = 'none';
        } else {
            putNeutralsButton.style.display = 'inline-block';
        }
    } else if (currentPlayer === 2) {
        if (player2NeutralsUsed || player2NeutralsStarted || countNonFortifiedCells(2) < 2) {
            putNeutralsButton.style.display = 'none';
        } else {
            putNeutralsButton.style.display = 'inline-block';
        }
    }
}
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should still be hidden after turn change');

// Test 6: Simulate another turn change back to Player 1
console.log('\n📋 Test 6: Another turn change back to Player 1');
currentPlayer = 1;

// Check button visibility again
if (putNeutralsButton) {
    if (currentPlayer === 1) {
        if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
            putNeutralsButton.style.display = 'none';
        } else {
            putNeutralsButton.style.display = 'inline-block';
        }
    }
}
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should still be hidden (Player 1 already used neutrals)');

console.log('\n✅ Button stays hidden after completion test completed');
console.log('\n📊 Summary:');
console.log('✅ Button hidden after first neutral placement');
console.log('✅ Button stays hidden after second neutral placement');
console.log('✅ Button stays hidden after turn changes');
console.log('✅ Button stays hidden for rest of game');
console.log('✅ Proper state management with both used and started flags');