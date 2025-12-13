# Task 8: Implement Connection Tree Visualization for Player Territory

## Core Concept: Visualizing Living Cell Connections

**Main Idea:** Add an optional visualization mode that draws connection lines from each player's base to all their living cells, forming a non-intersecting tree structure that clearly shows which cells are connected to the base.

**IMPORTANT: This is an Optional Feature**
- Users should be able to toggle between two visualization modes:
  - **Classic Mode** (default): Current visualization with just colored cells
  - **Connection Tree Mode** (new): Shows cells + connection lines forming trees
- Add a checkbox/toggle button in the UI to switch between modes
- Save user preference in localStorage to persist across sessions

**Why This Matters:**
1. **Visual Clarity**: Players can instantly see which cells are connected to their base
2. **Strategic Insight**: Easily identify vulnerable branches that could be cut off
3. **Beautiful Visualization**: Creates a more engaging and informative game board
4. **Educational Value**: Helps players understand the connectivity mechanic better
5. **User Choice**: Some players may prefer simpler visualization, others want more detail

## Current State vs Desired State

### Current Visualization
- Cells are rendered as colored squares with player symbols
- No visual indication of connection paths
- Players must mentally trace paths to verify connectivity
- Connection state is only checked when cells are isolated (turned neutral)

### Desired Visualization
- **Cells remain as colored squares** (keep existing rendering)
- **Add connection lines** drawn between adjacent cells of the same player
- **Lines form a tree structure** from the base to all connected cells
- **Dynamic updates**: Lines redraw when:
  - Player makes a move (new cell added)
  - Enemy captures a cell (tree branches may split)
  - Territory gets disconnected (dead branches disappear)

## Visual Design Requirements

### Line Rendering
1. **Where to Draw**: Lines should connect centers of adjacent cells
   - Use `canvas` overlay on top of the grid OR
   - Use SVG layer for cleaner vector graphics OR
   - Use CSS borders/pseudo-elements (simplest but limited)

2. **Line Style**:
   - Color: Match the player's color (slightly transparent to not overwhelm)
   - Thickness: 2-3px for visibility without clutter
   - Style: Solid lines (optional: animated flow effect for extra polish)

3. **Tree Structure Properties**:
   - **Non-intersecting**: Each cell has exactly ONE path to base
   - **Minimal spanning tree**: Avoid redundant connections
   - **Root**: Player's base cell
   - **Leaves**: Frontier cells (cells at the edge of territory)

### Visual Examples

```
Example 1: Simple Linear Connection
[B]---[1]---[1]---[1]
Base   |
       Connected cells forming a line

Example 2: Branching Tree
       [1]
        |
[B]---[1]---[1]
 |      |
[1]    [1]
Base with multiple branches

Example 3: After Disconnection
[B]---[1]---[1]    [1] [1]
 |                  ^   ^
[1]              Dead cells (no line to base)
Only cells with path to base have connecting lines
```

## Technical Implementation Strategy

### Approach 1: BFS-Based Tree Construction (RECOMMENDED)

**Algorithm:**
1. Start from player's base cell
2. Use BFS (Breadth-First Search) to traverse all connected cells
3. Track **parent** relationship: for each cell, remember which neighbor connected it to the base
4. Draw lines only along parent-child edges (forms a spanning tree)

**Pseudocode:**
```javascript
function buildConnectionTree(player) {
    const base = getPlayerBase(player);
    const tree = new Map(); // cell -> parent cell
    const queue = [base];
    const visited = new Set();
    visited.add(cellKey(base));
    tree.set(cellKey(base), null); // Base has no parent

    while (queue.length > 0) {
        const current = queue.shift();

        // Check all 8 neighbors
        for (const neighbor of getNeighbors(current)) {
            if (belongsToPlayer(neighbor, player) && !visited.has(cellKey(neighbor))) {
                visited.add(cellKey(neighbor));
                tree.set(cellKey(neighbor), current); // Parent is current
                queue.push(neighbor);
            }
        }
    }

    return tree;
}

function drawConnectionTree(player, tree) {
    // Clear previous lines for this player
    clearPlayerLines(player);

    // Draw line from each cell to its parent
    for (const [cellKey, parent] of tree.entries()) {
        if (parent !== null) { // Skip base (no parent)
            drawLine(parent, parseCell(cellKey), player);
        }
    }
}
```

### Approach 2: Canvas-Based Rendering

**HTML Structure:**
```html
<div id="game-container">
    <canvas id="connection-canvas"></canvas>
    <div id="game-board" class="board"></div>
</div>
```

**Canvas Setup:**
```javascript
const canvas = document.getElementById('connection-canvas');
const ctx = canvas.getContext('2d');

function initCanvas() {
    // Size canvas to match game board
    const cellSize = calculateCellSize();
    canvas.width = cols * cellSize;
    canvas.height = rows * cellSize;
    canvas.style.position = 'absolute';
    canvas.style.top = '0';
    canvas.style.left = '0';
    canvas.style.pointerEvents = 'none'; // Allow clicks to pass through
}

function drawLine(fromCell, toCell, player) {
    const cellSize = calculateCellSize();
    const fromX = fromCell.col * cellSize + cellSize / 2;
    const fromY = fromCell.row * cellSize + cellSize / 2;
    const toX = toCell.col * cellSize + cellSize / 2;
    const toY = toCell.row * cellSize + cellSize / 2;

    ctx.strokeStyle = getPlayerColor(player, 0.5); // 50% opacity
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(fromX, fromY);
    ctx.lineTo(toX, toY);
    ctx.stroke();
}

function clearAllLines() {
    ctx.clearRect(0, 0, canvas.width, canvas.height);
}
```

### Approach 3: SVG-Based Rendering (Alternative)

**Benefits:**
- Vector graphics (scales beautifully)
- Easier to animate individual lines
- Can add CSS styling to lines
- Better for complex effects

**Drawbacks:**
- More complex DOM manipulation
- Potentially slower with many lines (100+ cells)

**Example:**
```javascript
function drawLineSVG(fromCell, toCell, player) {
    const svg = document.getElementById('connection-svg');
    const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');

    const cellSize = calculateCellSize();
    line.setAttribute('x1', fromCell.col * cellSize + cellSize / 2);
    line.setAttribute('y1', fromCell.row * cellSize + cellSize / 2);
    line.setAttribute('x2', toCell.col * cellSize + cellSize / 2);
    line.setAttribute('y2', toCell.row * cellSize + cellSize / 2);
    line.setAttribute('stroke', getPlayerColor(player));
    line.setAttribute('stroke-width', '3');
    line.setAttribute('opacity', '0.5');
    line.classList.add(`player${player}-connection`);

    svg.appendChild(line);
}
```

## Integration Points

### When to Redraw Connection Trees

1. **After each move**: `handleMove()` in [script.js](script.js)
   ```javascript
   function handleMove(row, col) {
       // ... existing move logic ...

       // Redraw connection trees after move
       updateAllConnectionTrees();
   }
   ```

2. **After territory disconnection**: When cells lose connection to base
   ```javascript
   function checkAndRemoveDisconnected(player) {
       // ... existing disconnection logic ...

       // Redraw tree to reflect disconnected cells
       updateConnectionTree(player);
   }
   ```

3. **On board initialization**: `initGame()`, `newGame()`
   ```javascript
   function initGame() {
       // ... existing init logic ...

       renderBoard();
       updateAllConnectionTrees(); // NEW: Draw initial connections
   }
   ```

4. **On window resize**: `renderBoard()` recalculates cell size
   ```javascript
   window.addEventListener('resize', () => {
       renderBoard();
       updateAllConnectionTrees(); // Redraw with new cell size
   });
   ```

### Files to Modify

1. **[script.js](script.js)** (Primary changes)
   - Add `connectionTreeEnabled` global variable (default: false)
   - Add `buildConnectionTree(player)` function
   - Add `updateConnectionTree(player)` function
   - Add `updateAllConnectionTrees()` function (calls for all 4 players)
   - Add `drawConnectionLines(player, tree)` function
   - Add `toggleConnectionTreeMode()` function to enable/disable visualization
   - Modify `renderBoard()` to initialize canvas/SVG and check if mode is enabled
   - Modify `handleMove()` to call `updateConnectionTree(currentPlayer)` (only if enabled)
   - Modify `checkAndRemoveDisconnected()` to redraw trees (only if enabled)
   - Add localStorage save/load for user preference

2. **[index.html](index.html)** (UI changes)
   - Add `<canvas id="connection-canvas"></canvas>` element
   - Position canvas behind game board cells but above background
   - Add toggle checkbox/button for connection tree visualization:
     ```html
     <label>
       <input type="checkbox" id="connection-tree-enabled">
       <span data-translation-key="showConnectionTrees">Show Connection Trees</span>
     </label>
     ```

3. **[style.css](style.css)** (Styling)
   - Add styling for canvas container
   - Add styling for toggle checkbox/button
   - Ensure proper z-index layering:
     - Background (lowest)
     - Connection canvas (middle)
     - Game board cells (highest - must be clickable)
   - Add visual feedback for toggle state (enabled/disabled)

4. **[multiplayer.js](multiplayer.js)** (Multiplayer sync)
   - Ensure connection trees update when receiving moves from server
   - Call `updateAllConnectionTrees()` after applying opponent moves (check if enabled)

5. **[translations.js](translations.js)** (Localization)
   - Add translations for "Show Connection Trees" in all supported languages
   - Example keys:
     - `en`: "Show Connection Trees"
     - `ru`: "Показать деревья связей"
     - etc.

## Edge Cases to Handle

### 1. Multiple Players (2-4 players)
- Each player needs their own color-coded connection tree
- Trees can overlap visually (use transparency to handle this)
- Draw all trees on same canvas/SVG (layering by player number)

### 2. Base Destruction (If Implemented)
- If base is destroyed, all connections disappear
- Handle gracefully in `buildConnectionTree()`:
  ```javascript
  const base = getPlayerBase(player);
  if (!base || board[base.row][base.col] !== `${player}-base`) {
      return new Map(); // Empty tree, no connections
  }
  ```

### 3. Fortified Cells
- Fortified cells ARE part of the connection tree
- Draw connections to/through fortified cells normally
- Optionally: Make lines to fortified cells thicker/brighter

### 4. Diagonal Connections
- Game allows diagonal adjacency (8-directional)
- Lines will cross visually but that's OK
- Optionally: Use curved lines to reduce visual clutter

### 5. Large Boards (20x20+)
- Many cells = many lines = performance concern
- Optimize by:
  - Using single canvas.beginPath() for all lines
  - Only redrawing changed player's tree, not all 4
  - Using requestAnimationFrame for smooth updates

### 6. Neutral (Killed) Cells
- Dead cells have no connections
- When cell turns neutral, its branch is removed from tree
- Parent cell may still have other children (other branches)

### 7. Toggle State Management
- Connection tree mode disabled by default (classic visualization)
- When user toggles mode:
  - If enabling: Build and draw all connection trees
  - If disabling: Clear canvas and hide all connection lines
- Save preference to localStorage as `connectionTreeEnabled` (boolean)
- Load preference on page load:
  ```javascript
  function loadVisualizationPreference() {
      const saved = localStorage.getItem('connectionTreeEnabled');
      connectionTreeEnabled = saved === 'true';
      if (connectionTreeEnabled) {
          document.getElementById('connection-tree-enabled').checked = true;
          updateAllConnectionTrees();
      }
  }
  ```

## Performance Considerations

### Current Performance Budget
- BFS tree construction: O(N) where N = number of player cells
- Line drawing: O(N) where N = number of player cells
- For 10x10 board, max ~100 cells per player = fast
- For 20x20 board, max ~400 cells per player = still acceptable

### Optimization Strategies

1. **Incremental Updates** (Future Enhancement)
   - Instead of rebuilding entire tree, update only affected branch
   - When adding cell: extend tree with one edge
   - When removing cell: recalculate only affected subtree
   - More complex but faster for large boards

2. **Debounced Rendering**
   - Don't redraw during AI move animations
   - Queue updates and render once at end

3. **Canvas Optimization**
   - Batch all line draws in single `beginPath()`/`stroke()` call
   - Use `ctx.save()`/`ctx.restore()` to avoid repeated style setting

4. **Caching**
   - Cache tree structure between moves if territory unchanged
   - Only rebuild tree when player's territory changes

## Visual Polish (Optional Enhancements)

### Phase 1: Basic Implementation
- ✅ Solid colored lines
- ✅ BFS-based tree construction
- ✅ Redraw on move/disconnection

### Phase 2: Visual Enhancements (Future)
- 🎨 Animated "flow" effect (dashed lines moving toward base)
- 🎨 Gradient lines (darker near base, lighter at edges)
- 🎨 Curved lines for diagonals (smoother appearance)
- 🎨 Pulsing animation on newly added connections
- 🎨 Highlight critical "cut points" (cells whose removal splits tree)

### Phase 3: Advanced Features (Future)
- 🔍 Interactive: Hover over cell to highlight its path to base
- 📊 Show "distance to base" metric (number of hops)
- ⚡ Performance mode toggle (disable lines on large boards)
- 🎯 Strategic overlay: Show vulnerable connection points

## Testing Checklist

### Functional Tests
- [ ] **Toggle functionality works correctly**
  - [ ] Checkbox/button toggles between enabled and disabled states
  - [ ] Lines appear when mode is enabled
  - [ ] Lines disappear when mode is disabled
  - [ ] Preference is saved to localStorage
  - [ ] Preference is loaded on page reload
- [ ] **Classic mode (default)**: No lines visible, only colored cells
- [ ] **Connection tree mode**: Lines appear when enabled
- [ ] Lines connect all player cells to base
- [ ] Each cell has exactly one line to its parent (tree structure)
- [ ] No lines cross player boundaries (Player 1 lines ≠ Player 2 lines)
- [ ] Lines update when player makes a move (only if mode enabled)
- [ ] Lines disappear for disconnected cells
- [ ] Multiple players (3-4 player mode) show distinct colored trees

### Visual Tests
- [ ] Lines are visible but not overwhelming
- [ ] Lines don't obscure cell symbols
- [ ] Lines scale correctly with different board sizes (5x5 to 20x20)
- [ ] Canvas/SVG resizes correctly on window resize
- [ ] Lines align with cell centers (no off-by-one pixel errors)

### Performance Tests
- [ ] No lag when drawing connections for 10x10 board
- [ ] Acceptable performance on 20x20 board (test with all cells filled)
- [ ] Smooth updates during rapid moves (AI game)
- [ ] No memory leaks (check with long-running game)

### Edge Case Tests
- [ ] Works in multiplayer mode (2-4 players)
- [ ] Works with different board sizes (from settings)
- [ ] Works after loading saved game
- [ ] Works after window resize
- [ ] Handles base-less situation (if base destroyed)

## Success Criteria

1. ✅ **Clarity**: Players can visually see which cells are connected to base
2. ✅ **Accuracy**: Connection tree exactly matches `isConnectedToBase()` logic
3. ✅ **Performance**: No noticeable lag when drawing connections
4. ✅ **Aesthetics**: Lines enhance visual appeal without clutter
5. ✅ **Maintainability**: Code is clean, well-documented, and reusable

## Implementation Priority

**Priority: MEDIUM-HIGH** (Feature Enhancement, not a bug fix)

**Estimated Effort:**
- Phase 1 (Basic Canvas Implementation with Toggle): 4-6 hours
  - 1 hour: UI toggle setup (checkbox, localStorage, event handlers)
  - 1 hour: Canvas setup and integration
  - 2 hours: BFS tree construction algorithm
  - 1 hour: Line drawing and styling
  - 1 hour: Testing toggle functionality and edge cases

- Phase 2 (Visual Polish): 2-3 hours
  - Add animations, gradients, etc.

**Dependencies:**
- No blocking dependencies
- Can be implemented independently
- Works with current cell rendering system
- Compatible with Task 7 (if enum optimization is done, connection logic stays same)

## Notes for Implementation

1. **Start Simple**: Get basic toggle + canvas + BFS working first
2. **Test Incrementally**: Test tree construction separately from rendering
3. **Reuse Existing Logic**: `isConnectedToBase()` already does BFS, can adapt it
4. **Default to Classic Mode**: Keep current visualization as default, new mode is opt-in
5. **Respect User Choice**: Always check `connectionTreeEnabled` before drawing lines
6. **Performance**: Only update trees when mode is enabled (skip all work when disabled)
7. **Document Clearly**: Add comments explaining tree construction algorithm and toggle logic

## References to Existing Code

- **Connection Logic**: [script.js:15-57](script.js#L15-L57) - `isConnectedToBase()`
- **Board Rendering**: [script.js:107-146](script.js#L107-L146) - `renderBoard()`
- **Cell Size Calculation**: [script.js:93-105](script.js#L93-L105) - `calculateCellSize()`
- **Game Board Element**: [script.js:1](script.js#L1) - `gameBoard` variable

## Related Documentation

- [DOCS.md](DOCS.md) - Game rules and mechanics
- [ARCHITECTURE.md](backend/ARCHITECTURE.md) - System architecture
- [README.md](README.md) - Project overview

---

**Final Note**: This feature transforms the game board from a simple grid into a living, dynamic network visualization. It makes the core "connection to base" mechanic visually obvious and strategically informative. Players will instantly understand which territories are vulnerable and where cutting off enemy connections would be most effective.
