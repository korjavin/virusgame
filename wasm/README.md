# WebAssembly AI Implementation

This directory contains the Go implementation of the Virus Game AI, compiled to WebAssembly for improved performance.

## Performance Improvement

The WASM implementation provides **3-10x faster** AI calculations compared to JavaScript:
- At depth 3-4: ~3-5x faster
- At depth 5-6: ~5-10x faster

This allows for higher AI depths with acceptable response times.

## Building

### Local Build

```bash
cd wasm
./build.sh
```

This will:
1. Compile `ai.go` to `ai.wasm`
2. Copy `wasm_exec.js` from Go installation

### Docker Build

The WASM module is built automatically in the multi-stage Dockerfile:
- Stage 2 compiles the WASM module
- Stage 3 copies it to `/app/wasm/ai.wasm`

## Architecture

### Go Implementation (`ai.go`)

- **Minimax with Alpha-Beta Pruning**: Same algorithm as JavaScript version
- **Board Evaluation**: 4 criteria (material, mobility, position, attacks)
- **Move Validation**: Includes base connectivity checks
- **Progress Callbacks**: Updates UI during long calculations

### JavaScript Integration (`ai-wasm.js`)

- **Automatic Loading**: Initializes WASM on page load
- **Fallback Logic**: Uses JavaScript AI if WASM fails
- **Performance Logging**: Console logs show execution time
- **Transparent**: Drop-in replacement for `getAIMove()`

### Files

- `ai.go` - Go minimax implementation
- `go.mod` - Go module definition
- `build.sh` - Local build script
- `../ai-wasm.js` - JavaScript glue code
- `../wasm_exec.js` - Go WASM runtime (from Go stdlib)

## Usage

The WASM AI is automatically used when available. Check console for:

```
Loading WASM AI module...
WASM AI module loaded successfully!
Using WASM AI (depth: 3)
WASM AI took: 45.23 ms
```

If WASM fails to load, it automatically falls back to JavaScript:

```
Failed to load WASM AI module: ...
Falling back to JavaScript AI
Using JavaScript AI (depth: 3)
JS AI took: 234.56 ms
```

## Browser Compatibility

WebAssembly is supported in:
- Chrome 57+
- Firefox 52+
- Safari 11+
- Edge 16+

Older browsers will automatically use the JavaScript implementation.
