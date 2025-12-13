// Simple test to verify button behavior
console.log('🧪 Simple Button Behavior Test');

// Test the button visibility logic directly
function testButtonVisibility(currentPlayer, player1Used, player1Started, player2Used, player2Started, player1Cells, player2Cells) {
    let buttonVisible = 'inline-block';
    
    // Only show button for current player's turn
    if (currentPlayer === 1) {
        if (player1Used || player1Started || player1Cells < 2) {
            buttonVisible = 'none';
        } else {
            buttonVisible = 'inline-block';
        }
    } else if (currentPlayer === 2) {
        if (player2Used || player2Started || player2Cells < 2) {
            buttonVisible = 'none';
        } else {
            buttonVisible = 'inline-block';
        }
    } else {
        buttonVisible = 'none';
    }
    
    return buttonVisible;
}

// Test cases
console.log('\n📋 Test 1: Player 1 turn, has cells, not used');
const result1 = testButtonVisibility(1, false, false, false, false, 2, 0);
console.log('Result:', result1);
console.log('Expected: inline-block');
console.log('Status:', result1 === 'inline-block' ? '✅ PASS' : '❌ FAIL');

console.log('\n📋 Test 2: Player 2 turn, has cells, not used');
const result2 = testButtonVisibility(2, false, false, false, false, 0, 2);
console.log('Result:', result2);
console.log('Expected: inline-block');
console.log('Status:', result2 === 'inline-block' ? '✅ PASS' : '❌ FAIL');

console.log('\n📋 Test 3: Player 1 turn, but used neutrals');
const result3 = testButtonVisibility(1, true, false, false, false, 2, 0);
console.log('Result:', result3);
console.log('Expected: none');
console.log('Status:', result3 === 'none' ? '✅ PASS' : '❌ FAIL');

console.log('\n📋 Test 4: Player 1 turn, started but not completed');
const result4 = testButtonVisibility(1, false, true, false, false, 2, 0);
console.log('Result:', result4);
console.log('Expected: none');
console.log('Status:', result4 === 'none' ? '✅ PASS' : '❌ FAIL');

console.log('\n📋 Test 5: Player 1 turn, not enough cells');
const result5 = testButtonVisibility(1, false, false, false, false, 1, 0);
console.log('Result:', result5);
console.log('Expected: none');
console.log('Status:', result5 === 'none' ? '✅ PASS' : '❌ FAIL');

console.log('\n✅ Simple button test completed');