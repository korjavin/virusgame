import json
import asyncio
import websockets
import os
import sys
import random
from game import Game

# Configuration
BACKEND_URL = os.getenv("BACKEND_URL", "ws://localhost:8080/ws")

class Bot:
    def __init__(self, url):
        self.url = url
        self.ws = None
        self.user_id = None
        self.username = None
        self.game = None
        self.game_id = None
        self.your_player = None

    async def connect(self):
        try:
            async with websockets.connect(self.url) as websocket:
                self.ws = websocket
                print(f"Connected to {self.url}")
                await self.listen()
        except Exception as e:
            print(f"Connection error: {e}")

    async def listen(self):
        try:
            async for message in self.ws:
                await self.handle_message(json.loads(message))
        except websockets.exceptions.ConnectionClosed:
            print("Connection closed")

    async def handle_message(self, msg):
        msg_type = msg.get("type")

        if msg_type == "welcome":
            self.user_id = msg.get("userId")
            self.username = msg.get("username")
            print(f"Bot registered as {self.username}")

        elif msg_type == "bot_wanted":
            print(f"Bot requested for lobby {msg.get('lobbyId')}")
            await self.join_lobby(msg.get("lobbyId"))

        elif msg_type == "lobby_joined":
            print("Joined lobby")

        elif msg_type == "multiplayer_game_start":
            self.game_id = msg.get("gameId")
            self.your_player = msg.get("yourPlayer")
            rows = msg.get("rows")
            cols = msg.get("cols")
            self.game = Game(rows, cols, self.your_player)
            print(f"Game started, I am player {self.your_player}")

        elif msg_type == "turn_change":
            player = msg.get("player")
            moves_left = msg.get("movesLeft")

            # Sync our local state if needed (though we mostly track it via move_made)
            # But the most important thing is to move if it's our turn
            if player == self.your_player:
                print(f"My turn! Moves left: {moves_left}")
                await self.make_moves(moves_left)

        elif msg_type == "move_made":
            player = msg.get("player")
            row = msg.get("row")
            col = msg.get("col")
            if row is not None and col is not None:
                self.game.apply_move(row, col, player)

        elif msg_type == "game_end":
            print(f"Game ended, winner: {msg.get('winner')}")
            self.game = None
            self.game_id = None

    async def join_lobby(self, lobby_id):
        await self.send({
            "type": "join_lobby",
            "lobbyId": lobby_id
        })

    async def make_moves(self, moves_left):
        # Make as many moves as we are allowed/can
        current_moves = moves_left
        while current_moves > 0:
            move = self.find_move()
            if move:
                row, col = move
                print(f"Sending move: {row}, {col}")
                await self.send({
                    "type": "move",
                    "gameId": self.game_id,
                    "row": row,
                    "col": col
                })
                # We optimistically update our board or wait for confirmation?
                # The protocol says we receive 'move_made'.
                # Ideally we wait a bit or just send all moves.
                # However, the server might process them sequentially.
                # For this simple bot, let's just wait a tiny bit to avoid flooding if logic is fast
                await asyncio.sleep(0.1)
                current_moves -= 1
            else:
                print("No valid moves found")
                break

    def find_move(self):
        # Try to find a valid move
        # Simple random strategy
        rows = self.game.rows
        cols = self.game.cols

        # Create a list of all coordinates and shuffle them
        coords = [(r, c) for r in range(rows) for c in range(cols)]
        random.shuffle(coords)

        for r, c in coords:
            if self.game.is_valid_move(r, c, self.your_player):
                return (r, c)

        return None

    async def send(self, data):
        if self.ws:
            await self.ws.send(json.dumps(data))

if __name__ == "__main__":
    bot = Bot(BACKEND_URL)
    asyncio.run(bot.connect())
