#!/bin/bash
set -e

echo "Building WASM module..."
GOOS=js GOARCH=wasm go build -o ai.wasm ai.go

echo "Copying wasm_exec.js..."
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" ../wasm_exec.js

echo "WASM build complete!"
ls -lh ai.wasm
