# Continuity Ledger

## Goal
**Implement vs-ai2.26 Halo Dominance Policy**
- Avoid redundant halo placements during verified no-contact opening.
- Allow forced first halo exit and sole preserving halo placements.
- Preserve tactical search elements (wins, captures, eliminations, opponent-base cuts, neutrals, sole legal, sole preserving, and forced defense).
- Identical behavior under serial and parallel search, applicable to all seats and rectangles.
- Allocation-free contact/threat/cut analysis using reusable workspace storage.

## Constraints/Assumptions
- **Opening Mode**: Verified no-contact opening, i.e., no actor-connected cell is adjacent to any opponent Normal or Fortified cell.
- **Halo definition**: The 8-neighbor ring around the actor's own base.
- **Halo move**: An empty Normal placement targeting a halo cell.
- **Outward action**: Any preserving action that is not a halo move.
- **Ordinal stability**: Preserved deterministic ordinal indices.

## Key Decisions
- Implement the policy inside `orderedChildren` for `root == true` to seamlessly cover both `atDepth` (serial) and `atDepthParallel` (parallel) searches.
- Maintain `s.contactSeen`, `s.contactSeen2`, and `s.contactQueue` in the `searcher` struct allocated lazily for root searches to keep worker searches completely allocation-free.
- Exclude/suppress dominated halo actions by returning `true` (continue) from the search action loop, keeping the original ordinals intact.

## State
- **Done**:
  - Implemented contact detection BFS (`hasContact`) using reusable workspace.
  - Implemented opponent-base cut check (`isOpponentBaseCut`) and actor threatened cells counter (`isForcedDefense`) using workspace.
  - Implemented the dominance rule in `orderedChildren` with all the required exceptions.
  - Verified compilation and correctness via the full unit test suite including race detection and allocations.
  - Added focused unit tests in `TestHaloDominancePolicy`.
- **Now**:
  - Committing and pushing the branch `agy/vs-ai2.26-halo-policy`.
  - Opening the draft PR.
- **Next**:
  - Report exact results.

## Open Questions
- None.

## Working Set
- `backend/search/search.go`
- `backend/search/search_test.go`