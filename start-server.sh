#!/bin/bash

# Start the Virus Game multiplayer server

echo "Starting Virus Game Multiplayer Server..."
echo ""
echo "The server will be available at: http://localhost:8080"
echo "Open this URL in multiple browser windows to test multiplayer!"
echo ""
echo "Press Ctrl+C to stop the server"
echo ""

cd backend
go run .
