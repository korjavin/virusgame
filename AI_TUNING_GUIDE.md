# AI Tuning Guide

The AI evaluation function now has tunable coefficients that you can adjust in the web UI to experiment with different AI behaviors.

## How to Access

1. Enable "vs AI" checkbox
2. The "⚙️ AI Coefficients" section will appear
3. Click the header to expand/collapse the controls

## Coefficients Explained

### Cell Value (default: 10)
- **What it does**: Base score for each cell controlled
- **Higher values**: AI prioritizes controlling more territory
- **Lower values**: AI focuses less on raw territory count
- **Try**: 5-20 range

### Fortified Value (default: 15)
- **What it does**: Extra points for fortified cells (captured enemy cells)
- **Higher values**: AI becomes more aggressive, prioritizes attacks
- **Lower values**: AI prefers expansion over attacking
- **Try**: 0-30 range

### Mobility Value (default: 5)
- **What it does**: Points per available move (flexibility)
- **Higher values**: AI keeps options open, avoids getting trapped
- **Lower values**: AI makes more committal moves
- **Try**: 0-15 range

### Aggression Value (default: 1)
- **What it does**: Multiplier for distance to opponent's base
- **Higher values**: AI rushes toward opponent aggressively
- **Lower values**: AI plays more defensively/cautiously
- **Try**: 0-3 range

### Connection Value (default: 3)
- **What it does**: Points for each adjacent friendly cell
- **Higher values**: AI builds thick, connected networks
- **Lower values**: AI spreads out more, takes risks
- **Try**: 0-10 range

### Attack Value (default: 8)
- **What it does**: Points for cells adjacent to enemy (attack opportunities)
- **Higher values**: AI seeks confrontation
- **Lower values**: AI avoids enemy contact
- **Try**: 0-20 range

## Experiment Ideas

### Aggressive AI
```
Cell: 5
Fortified: 30
Mobility: 3
Aggression: 2
Connection: 1
Attack: 20
```

### Defensive AI
```
Cell: 15
Fortified: 5
Mobility: 10
Aggression: 0.5
Connection: 8
Attack: 2
```

### Balanced AI (default)
```
Cell: 10
Fortified: 15
Mobility: 5
Aggression: 1
Connection: 3
Attack: 8
```

### Expansion-Focused
```
Cell: 20
Fortified: 5
Mobility: 8
Aggression: 1.5
Connection: 2
Attack: 5
```

## Tips for Tuning

1. **Change one at a time**: Adjust one coefficient, play a game, observe behavior
2. **Relative values matter**: It's the ratio between coefficients, not absolute values
3. **Watch for extremes**: Setting any value to 0 disables that factor entirely
4. **Depth matters**: Higher depths (4-6) show coefficient effects more clearly
5. **Board size affects**: Larger boards make positional play (aggression, connection) more important

## Finding the Best AI

Play multiple games with different settings and note:
- Does AI win/lose?
- Does AI make interesting moves?
- Does AI adapt to your strategy?
- Is AI too predictable?

The "best" AI depends on what you want:
- **Challenging**: Balanced with high mobility
- **Fun**: Moderate aggression, some risk-taking
- **Unpredictable**: Lower connection, higher aggression variation

## Saving Your Settings

Settings persist during your session but reset on page reload. If you find coefficients you like, note them down!

## Technical Notes

The evaluation function calculates a score for each board position by:
1. Counting material (cells × coefficient)
2. Assessing mobility (moves × coefficient)
3. Evaluating positions (distance, connections)
4. Scoring attack opportunities

Minimax then explores the game tree to depth N, choosing moves that maximize this score while assuming the opponent minimizes it.
