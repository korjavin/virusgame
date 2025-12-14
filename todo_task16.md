# Visual Fix: Connection Lines Obscuring Triangle Symbol

There is a visual issue when playing as the player represented by the Triangle symbol ('△'). The connection lines drawn to indicate territory or pathing are currently rendering in a way that fully obscures or looks messy when crossing the triangle symbol.

## Problem:
*   Connection lines drawn inside or over the '△' symbol make it difficult to see or aesthetically unpleasing.
*   The lines "clash" with the filled area or the outline of the triangle, effectively hiding it.

## Requirements:
*   Adjust the rendering of connection lines so they do not obscure the white triangle symbol.
*   The triangle symbol must remain clearly visible and distinct, even when connection lines pass through its cell.

## Implementation Ideas:
*   **Layering**: Ensure the canvas drawing the connection lines is positioned *behind* the text/symbol layer of the game cells in the DOM stack (z-index check).
*   **Masking**: If on the same layer or if layering isn't sufficient, implement a mask or "exclusion zone" in the center of the cell where the symbol resides, preventing lines from being drawn directly over the character.
*   **Opacity/Blending**: Adjust the transparency or blending mode of the connection lines to ensure the bright white symbol stands out against them.

## Implementation Notes:
*   Review `script.js` drawing functions (`drawConnectionTree`, etc.).
*   Check `style.css` for z-index layering of `#game-board`, `.cell`, and `#connection-canvas`.
