class Game {
    static get CellFlag() {
        return {
            NORMAL: 0x00,
            BASE: 0x10,
            FORTIFIED: 0x20,
            KILLED: 0x30
        };
    }

    static get EMPTY() { return 0x00; }
    static get FLAG_MASK() { return 0x30; }
    static get PLAYER_MASK() { return 0x0F; }

    constructor(rows, cols, myPlayerId) {
        this.rows = rows;
        this.cols = cols;
        this.myPlayerId = myPlayerId;

        // Initialize board
        this.board = Array(rows).fill(0).map(() => Array(cols).fill(Game.EMPTY));

        this.player1Base = { row: 0, col: 0 };
        this.player2Base = { row: rows - 1, col: cols - 1 };

        this.board[this.player1Base.row][this.player1Base.col] = this.createCell(1, Game.CellFlag.BASE);
        this.board[this.player2Base.row][this.player2Base.col] = this.createCell(2, Game.CellFlag.BASE);

        this.playerBases = {
            1: this.player1Base,
            2: this.player2Base
        };
    }

    createCell(player, flag = Game.CellFlag.NORMAL) {
        return (flag | player);
    }

    getPlayer(cell) {
        return cell & Game.PLAYER_MASK;
    }

    getFlag(cell) {
        return cell & Game.FLAG_MASK;
    }

    isBase(cell) {
        return (cell & Game.FLAG_MASK) === Game.CellFlag.BASE;
    }

    isFortified(cell) {
        return (cell & Game.FLAG_MASK) === Game.CellFlag.FORTIFIED;
    }

    canBeAttacked(cell) {
        return (cell & Game.FLAG_MASK) === Game.CellFlag.NORMAL;
    }

    applyMove(row, col, player) {
        const cellValue = this.board[row][col];

        if (cellValue === Game.EMPTY) {
            this.board[row][col] = this.createCell(player, Game.CellFlag.NORMAL);
        } else {
             if (this.canBeAttacked(cellValue) && this.getPlayer(cellValue) !== player) {
                 this.board[row][col] = this.createCell(player, Game.CellFlag.FORTIFIED);
             }
        }
    }

    isValidMove(row, col, player) {
        const cellValue = this.board[row][col];

        if (cellValue !== Game.EMPTY) {
            if (!this.canBeAttacked(cellValue)) {
                return false;
            }
            if (this.getPlayer(cellValue) === player) {
                return false;
            }
        }

        for (let i = -1; i <= 1; i++) {
            for (let j = -1; j <= 1; j++) {
                if (i === 0 && j === 0) continue;

                const adjRow = row + i;
                const adjCol = col + j;

                if (adjRow >= 0 && adjRow < this.rows && adjCol >= 0 && adjCol < this.cols) {
                    const adjCellValue = this.board[adjRow][adjCol];
                    if (adjCellValue !== Game.EMPTY &&
                        this.getPlayer(adjCellValue) === player &&
                        this.isConnectedToBase(adjRow, adjCol, player)) {
                        return true;
                    }
                }
            }
        }
        return false;
    }

    isConnectedToBase(startRow, startCol, player) {
        const base = this.playerBases[player];
        if (!base) return false;

        const visited = new Set();
        const stack = [{ row: startRow, col: startCol }];
        visited.add(`${startRow},${startCol}`);

        while (stack.length > 0) {
            const { row, col } = stack.pop();

            if (row === base.row && col === base.col) {
                return true;
            }

            for (let i = -1; i <= 1; i++) {
                for (let j = -1; j <= 1; j++) {
                    if (i === 0 && j === 0) continue;

                    const newRow = row + i;
                    const newCol = col + j;

                    if (newRow >= 0 && newRow < this.rows && newCol >= 0 && newCol < this.cols) {
                        const key = `${newRow},${newCol}`;
                        if (!visited.has(key)) {
                            const cellValue = this.board[newRow][newCol];
                            if (cellValue !== Game.EMPTY && this.getPlayer(cellValue) === player) {
                                visited.add(key);
                                stack.push({ row: newRow, col: newCol });
                            }
                        }
                    }
                }
            }
        }
        return false;
    }
}

module.exports = Game;
