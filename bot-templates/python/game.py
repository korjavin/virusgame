class Game:
    # Cell Constants (mirroring script.js)
    CELL_FLAG_NORMAL = 0x00
    CELL_FLAG_BASE = 0x10
    CELL_FLAG_FORTIFIED = 0x20
    CELL_FLAG_KILLED = 0x30

    EMPTY = 0x00
    FLAG_MASK = 0x30
    PLAYER_MASK = 0x0F

    def __init__(self, rows, cols, my_player_id):
        self.rows = rows
        self.cols = cols
        self.my_player_id = my_player_id
        # Initialize board with 0 (EMPTY)
        self.board = [[self.EMPTY for _ in range(cols)] for _ in range(rows)]

        # Base locations (standard setup)
        # Note: In a real implementation, we might want to wait for initial board state
        # or deduce bases, but standard rules put P1 at (0,0) and P2 at (rows-1, cols-1)
        self.player1_base = (0, 0)
        self.player2_base = (rows - 1, cols - 1)

        # Initialize bases on board
        self.board[self.player1_base[0]][self.player1_base[1]] = self.create_cell(1, self.CELL_FLAG_BASE)
        self.board[self.player2_base[0]][self.player2_base[1]] = self.create_cell(2, self.CELL_FLAG_BASE)

        # Track bases for is_connected_to_base check
        self.player_bases = {
            1: self.player1_base,
            2: self.player2_base
        }

    @staticmethod
    def create_cell(player, flag=CELL_FLAG_NORMAL):
        return flag | player

    @staticmethod
    def get_player(cell):
        return cell & Game.PLAYER_MASK

    @staticmethod
    def get_flag(cell):
        return cell & Game.FLAG_MASK

    @staticmethod
    def is_base(cell):
        return (cell & Game.FLAG_MASK) == Game.CELL_FLAG_BASE

    @staticmethod
    def is_fortified(cell):
        return (cell & Game.FLAG_MASK) == Game.CELL_FLAG_FORTIFIED

    @staticmethod
    def is_killed(cell):
        return (cell & Game.FLAG_MASK) == Game.CELL_FLAG_KILLED

    @staticmethod
    def can_be_attacked(cell):
        # Only normal cells can be attacked (not base, fortified, or killed)
        return (cell & Game.FLAG_MASK) == Game.CELL_FLAG_NORMAL

    def apply_move(self, row, col, player):
        cell_value = self.board[row][col]

        if cell_value == self.EMPTY:
             self.board[row][col] = self.create_cell(player, self.CELL_FLAG_NORMAL)
        else:
            # If it's an attack on an opponent
            if self.can_be_attacked(cell_value) and self.get_player(cell_value) != player:
                 self.board[row][col] = self.create_cell(player, self.CELL_FLAG_FORTIFIED)
            # Note: The server sends 'move_made' for valid moves.
            # If we receive it, we assume it happened.
            # We don't strictly need to re-validate here, just update state.

    def is_valid_move(self, row, col, player):
        cell_value = self.board[row][col]

        # Check if cell is occupied and not attackable
        if cell_value != self.EMPTY:
            if not self.can_be_attacked(cell_value):
                return False

            # Cannot place on own cell
            if self.get_player(cell_value) == player:
                return False

        # Check if adjacent to own territory connected to base
        # Iterate neighbors
        for i in range(-1, 2):
            for j in range(-1, 2):
                if i == 0 and j == 0:
                    continue

                adj_row = row + i
                adj_col = col + j

                if 0 <= adj_row < self.rows and 0 <= adj_col < self.cols:
                    adj_cell_value = self.board[adj_row][adj_col]
                    if (adj_cell_value != self.EMPTY and
                        self.get_player(adj_cell_value) == player and
                        self.is_connected_to_base(adj_row, adj_col, player)):
                        return True

        return False

    def is_connected_to_base(self, start_row, start_col, player):
        base = self.player_bases.get(player)
        if not base:
            return False

        visited = set()
        stack = [(start_row, start_col)]
        visited.add((start_row, start_col))

        while stack:
            r, c = stack.pop()

            if r == base[0] and c == base[1]:
                return True

            for i in range(-1, 2):
                for j in range(-1, 2):
                    if i == 0 and j == 0:
                        continue

                    new_row = r + i
                    new_col = c + j

                    if (0 <= new_row < self.rows and 0 <= new_col < self.cols and
                        (new_row, new_col) not in visited):

                        cell_val = self.board[new_row][new_col]
                        if cell_val != self.EMPTY and self.get_player(cell_val) == player:
                            visited.add((new_row, new_col))
                            stack.append((new_row, new_col))

        return False
