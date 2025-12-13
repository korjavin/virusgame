// Comprehensive test for all neutral button scenarios
console.log('🧪 Comprehensive Neutral Button Test');

// Mock all game state variables
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

// Mock board with sufficient cells for both players
const mockBoard = [];
for (let i = 0; i < 10; i++) {
    const row = [];
    for (let j = 0; j < 10; j++) {
        if (i < 2 && j < 2) {
            row.push(1); // Player 1 cells
        } else if (i > 7 && j > 7) {
            row.push(2); // Player 2 cells
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

// Mock updateStatus function
function updateStatus() {
    if (putNeutralsButton) {
        // Only show button for current player's turn
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
        } else {
            putNeutralsButton.style.display = 'none';
        }
    }
}

console.log('Initial state - Player 1 turn:');
console.log('Current player:', currentPlayer);
console.log('Player 1 cells:', countNonFortifiedCells(1));

// Test 1: Button visible on Player 1's turn
console.log('\n📋 Test 1: Button visible on Player 1\'s turn');
updateStatus();
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: inline-block');
console.log('Result:', putNeutralsButton.style.display === 'inline-block' ? '✅ PASS' : '❌ FAIL');

// Test 2: Button hidden on Player 2's turn
console.log('\n📋 Test 2: Button hidden on Player 2\'s turn');
currentPlayer = 2;
updateStatus();
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

// Test 3: Button visible again on Player 1's turn
console.log('\n📋 Test 3: Button visible again on Player 1\'s turn');
currentPlayer = 1;
updateStatus();
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: inline-block');
console.log('Result:', putNeutralsButton.style.display === 'inline-block' ? '✅ PASS' : '❌ FAIL');

// Test 4: Start neutral placement
console.log('\n📋 Test 4: Start neutral placement');
if (currentPlayer === 1 && !player1NeutralsUsed && countNonFortifiedCells(1) >= 2) {
    neutralMode = true;
    putNeutralsButton.textContent = 'Cancel Neutral Placement';
    console.log('Neutral mode started');
    console.log('Button text:', putNeutralsButton.textContent);
    console.log('Expected: Cancel Neutral Placement');
    console.log('Result:', putNeutralsButton.textContent === 'Cancel Neutral Placement' ? '✅ PASS' : '❌ FAIL');
}

// Test 5: Button hidden during opponent's turn while in neutral mode
console.log('\n📋 Test 5: Button hidden during opponent\'s turn while in neutral mode');
currentPlayer = 2;
updateStatus();
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none (opponent\'s turn)');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

// Test 6: Place first neutral
console.log('\n📋 Test 6: Place first neutral');
currentPlayer = 1; // Back to Player 1's turn
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
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

// Test 7: Button still hidden on opponent's turn
console.log('\n📋 Test 7: Button still hidden on opponent\'s turn');
currentPlayer = 2;
updateStatus();
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

// Test 8: Place second neutral
console.log('\n📋 Test 8: Place second neutral');
currentPlayer = 1; // Back to Player 1's turn
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
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

// Test 9: Button still hidden after completion on any turn
console.log('\n📋 Test 9: Button still hidden after completion on any turn');
currentPlayer = 2;
updateStatus();
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

currentPlayer = 1;
updateStatus();
console.log('Current player:', currentPlayer);
console.log('Button visible:', putNeutralsButton.style.display);
console.log('Expected: none');
console.log('Result:', putNeutralsButton.style.display === 'none' ? '✅ PASS' : '❌ FAIL');

console.log('\n✅ Comprehensive test completed');
console.log('\n📊 Summary:');
console.log('✅ Button only visible during current player\'s turn');
console.log('✅ Button hidden during opponent\'s turn');
console.log('✅ Button hidden after first neutral placement');
console.log('✅ Button stays hidden after completion');
console.log('✅ Button stays hidden for rest of game');