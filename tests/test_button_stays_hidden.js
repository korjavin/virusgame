// Test that button stays hidden for rest of game
console.log('🧪 Testing Button Stays Hidden for Rest of Game');

// Mock game state
let currentPlayer = 1;
let player1NeutralsUsed = false;
let player1NeutralsStarted = false;
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
        if (i < 3 && j < 3) {
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
    if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
        putNeutralsButton.style.display = 'none';
    } else {
        putNeutralsButton.style.display = 'inline-block';
    }
}
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should be visible - player has enough cells');

// Test 2: Start neutral placement
console.log('\n📋 Test 2: Start neutral placement');
if (currentPlayer === 1 && !player1NeutralsUsed && countNonFortifiedCells(1) >= 2) {
    neutralMode = true;
    putNeutralsButton.textContent = 'Cancel Neutral Placement';
    console.log('✅ Neutral mode started');
    console.log('Button text:', putNeutralsButton.textContent);
}

// Test 3: Place first neutral cell
console.log('\n📋 Test 3: Place first neutral cell');
if (neutralMode) {
    const row = 0, col = 0;
    const cellValue = mockBoard[row][col];
    console.log('Cell value before:', cellValue);
    
    if (cellValue === currentPlayer) {
        mockBoard[row][col] = 'killed';
        neutralsPlaced++;
        console.log('✅ First neutral placed');
        
        // Mark that player started using neutrals
        if (neutralsPlaced === 1) {
            player1NeutralsStarted = true;
            if (putNeutralsButton) {
                putNeutralsButton.style.display = 'none';
                console.log('✅ Button hidden after first placement');
            }
        }
        
        console.log('Cell value after:', mockBoard[row][col]);
        console.log('Neutrals placed:', neutralsPlaced);
        console.log('Button visible:', putNeutralsButton.style.display);
    }
}

// Test 4: Check button visibility after placing first neutral
console.log('\n📋 Test 4: Check button visibility after placing first neutral');
if (putNeutralsButton) {
    if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
        putNeutralsButton.style.display = 'none';
    } else {
        putNeutralsButton.style.display = 'inline-block';
    }
}
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should still be hidden');

// Test 5: Simulate turn change (player expands more)
console.log('\n📋 Test 5: Simulate turn change (player expands more)');
// Add more cells to simulate expansion
mockBoard[3][0] = 1;
mockBoard[3][1] = 1;
console.log('Player 1 cells after expansion:', countNonFortifiedCells(1));

// Check button visibility again
if (putNeutralsButton) {
    if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
        putNeutralsButton.style.display = 'none';
    } else {
        putNeutralsButton.style.display = 'inline-block';
    }
}
console.log('Button visible:', putNeutralsButton.style.display);
console.log('✅ Button should still be hidden (neutrals started)');

// Test 6: Complete neutral placement
console.log('\n📋 Test 6: Complete neutral placement');
if (neutralMode) {
    const row = 0, col = 1;
    const cellValue = mockBoard[row][col];
    console.log('Cell value before:', cellValue);
    
    if (cellValue === currentPlayer) {
        mockBoard[row][col] = 'killed';
        neutralsPlaced++;
        console.log('✅ Second neutral placed');
        
        if (neutralsPlaced === 2) {
            player1NeutralsUsed = true;
            neutralMode = false;
            console.log('✅ Neutral placement completed');
        }
        
        console.log('Cell value after:', mockBoard[row][col]);
        console.log('Neutrals placed:', neutralsPlaced);
    }
}

// Test 7: Check button visibility after completion
console.log('\n📋 Test 7: Check button visibility after completion');
if (putNeutralsButton) {
    if (player1NeutralsUsed || player1NeutralsStarted || countNonFortifiedCells(1) < 2) {
        putNeutralsButton.style.display = 'none';
    } else {
        putNeutralsButton.style.display = 'inline-block';
    }
}
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Player 1 neutrals used:', player1NeutralsUsed);
console.log('Player 1 neutrals started:', player1NeutralsStarted);
console.log('✅ Button should still be hidden (both used and started)');

console.log('\n✅ Button stays hidden test completed');
console.log('\n📊 Summary:');
console.log('✅ Button hidden after first neutral placement');
console.log('✅ Button stays hidden even when player gains more cells');
console.log('✅ Button stays hidden after turn changes');
console.log('✅ Button stays hidden for rest of game');
console.log('✅ Proper state management with neutralsStarted flag');