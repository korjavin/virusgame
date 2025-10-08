// WASM AI wrapper for Virus Game
// Loads and interfaces with the Go WebAssembly AI module

let wasmAIReady = false;
let wasmModule = null;

// Initialize WASM module
async function initWasmAI() {
    try {
        console.log('Loading WASM AI module...');

        // Load the wasm_exec.js glue code
        const go = new Go();

        // Load the WASM module
        const result = await WebAssembly.instantiateStreaming(
            fetch('wasm/ai.wasm'),
            go.importObject
        );

        // Run the Go program
        go.run(result.instance);

        // Wait for WASM to signal it's ready
        await waitForWasmReady();

        wasmAIReady = true;
        console.log('WASM AI module loaded successfully!');

    } catch (error) {
        console.error('Failed to load WASM AI module:', error);
        console.log('Falling back to JavaScript AI');
        wasmAIReady = false;
    }
}

function waitForWasmReady() {
    return new Promise((resolve) => {
        const checkReady = () => {
            if (window.wasmReady) {
                resolve();
            } else {
                setTimeout(checkReady, 100);
            }
        };
        checkReady();
    });
}

// Callback for WASM to update progress
window.updateAIProgressFromWasm = function(current, total) {
    aiProgressCurrent = current;
    aiProgressTotal = total;
    updateAIProgress();
};

// Get AI move using WASM
function getAIMoveWasm() {
    if (!wasmAIReady || !window.wasmGetAIMove) {
        console.warn('WASM not ready, falling back to JS AI');
        return getAIMoveJS();
    }

    try {
        console.log('WASM AI: Calling with bases:', {
            player1: {row: player1Base.row, col: player1Base.col},
            player2: {row: player2Base.row, col: player2Base.col},
            rows, cols, depth: aiDepth
        });

        const move = window.wasmGetAIMove(
            board,
            rows,
            cols,
            aiDepth,
            player1Base.row,
            player1Base.col,
            player2Base.row,
            player2Base.col
        );

        if (!move) {
            console.warn('WASM returned null move');
            return null;
        }

        console.log('WASM AI: Selected move:', move);

        return {
            row: move.row,
            col: move.col,
            score: move.score
        };
    } catch (error) {
        console.error('WASM AI error:', error);
        console.log('Falling back to JavaScript AI');
        return getAIMoveJS();
    }
}

// Backup JavaScript AI (rename the original function)
function getAIMoveJS() {
    if (gameOver || currentPlayer !== 2) {
        return null;
    }

    // Reset progress tracking
    const possibleMoves = getAllValidMoves(board, 2);
    console.log('JS AI: Found', possibleMoves.length, 'valid moves. First 5:', possibleMoves.slice(0, 5));

    aiProgressCurrent = 0;
    aiProgressTotal = possibleMoves.length;
    updateAIProgress();

    // Use minimax to find the best move
    const result = minimax(board, aiDepth, -Infinity, Infinity, true, true);

    console.log('JS AI: Selected move:', result.move, 'Score:', result.score);

    // Hide progress indicator
    hideAIProgress();

    return result.move;
}

// Override the original getAIMove to use WASM if available
const originalGetAIMove = getAIMove;
getAIMove = function() {
    // Enable WASM for comparison testing
    const useWASM = true; // Set to false to disable WASM

    if (wasmAIReady && useWASM) {
        console.log('Using WASM AI (depth:', aiDepth, ')');
        const startTime = performance.now();
        const move = getAIMoveWasm();
        const duration = performance.now() - startTime;
        console.log('WASM AI took:', duration.toFixed(2), 'ms');
        hideAIProgress();
        return move;
    } else {
        console.log('Using JavaScript AI (depth:', aiDepth, ')');
        const startTime = performance.now();
        const move = getAIMoveJS();
        const duration = performance.now() - startTime;
        console.log('JS AI took:', duration.toFixed(2), 'ms');
        return move;
    }
};

// Initialize WASM when page loads
if (typeof window !== 'undefined') {
    window.addEventListener('DOMContentLoaded', () => {
        initWasmAI();
    });
}
