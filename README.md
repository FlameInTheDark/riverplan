# River Plan Optimizer

## Overview

This application is a helper tool, specifically a River Planner, for the game "Loop Hero." River Plan Optimizer is a Go application built with the Ebitengine 2D game library. Its purpose is to help users find the optimal placement of "River" and "Forest" tiles on a 12x21 grid to maximize an attack speed bonus, referred to as "profit." The user defines a road layout and a river starting point, and the application calculates the best river path and corresponding forest placements.

## Core Logic (`game` package)

The core game logic revolves around a grid system and specific rules for placing different types of tiles.

### Tile Types

The grid can contain the following types of tiles:
*   `Empty`: Default state, available for building rivers or forests.
*   `Road`: Placed by the user.
*   `River`: Placed by the pathfinding algorithm.
*   `Forest`: Placed adjacent to river tiles.
*   `Forbidden`: Tiles adjacent (Up, Down, Left, Right) to `Road` tiles become `Forbidden` and cannot be built upon.

### Road Placement

*   The user interactively places `Road` tiles on the grid.
*   Tiles directly adjacent (not diagonally) to any `Road` tile automatically become `Forbidden`.

### River Source Selection

*   After finalizing the road layout, the user selects a valid `Empty` tile on the border of the grid (excluding corners) to serve as the river's origin.

### River Pathfinding (`exploreAndEvaluateRecursive`)

The application employs a recursive pathfinding algorithm to determine the optimal river path.
*   **Adjacency**: Subsequent `River` tiles must be placed adjacent (Up, Down, Left, Right) to the previous river tile.
*   **Empty Tiles Only**: Rivers can only be placed on `Empty` tiles.
*   **Max Length**: The maximum length of the river is user-adjustable (default 35, minimum 5, maximum 35).
*   **No U-Turns**: Rivers cannot make immediate U-turns (e.g., if a river flows A -> B -> C, the next segment D cannot be A).
*   **Cross-River Adjacency (Toggleable)**: A feature (`DisableCrossRiverAdjacency`) can be enabled to prevent the river from being placed next to any part of itself, except for the segment immediately preceding it. This helps create more spaced-out river paths.
*   **Pathfinding Heuristics**: The algorithm uses a sophisticated heuristic to guide its search for the most profitable path. The current heuristic prioritizes moves based on:
    1.  **Adjacency Bonus**: Higher scores are given to moves that lead to potential forest spots (neighbors of the next river tile) being adjacent to more existing river segments.
    2.  **New Forest Tiles Count**: Higher scores are given if the next river tile placement opens up more `Empty` neighboring tiles for potential forest placement.
    3.  **Turns Preferred over Straights**: If the above scores are equal, the algorithm prefers to make a turn rather than continue straight. This was found to often lead to more compact and profitable river/forest formations.
    *   The pathfinding also prefers to build on non-border tiles if available, resorting to border tiles only when no non-border options exist.

### Forest Placement

*   `Forest` tiles are automatically placed on any `Empty` tile that is adjacent (Up, Down, Left, Right) to a `River` tile once a river path is determined.

### Profit Calculation

The "profit" (attack speed bonus) is calculated based on the placement of `Forest` tiles and their adjacency to `River` tiles:
*   Each `Forest` tile has a base profit contribution (e.g., 2%).
*   This base profit is multiplied by `(2 * NumberOfAdjacentRiverTiles)`. For example, a forest tile adjacent to 1 river segment gets `BaseProfit * 2`, a forest tile adjacent to 2 river segments gets `BaseProfit * 4`, and so on.
*   The total profit for a given river path is the sum of the profits from all placed `Forest` tiles.

### Iterative Length Calculation

To find the true optimal solution, the system iterates through possible river lengths:
*   It starts from a defined `minRiverLength` (e.g., 5) and goes up to the user-selected maximum length.
*   For each length in this range, it performs a full `FindOptimalRiverAndForests` search.
*   It keeps track of the `overallBestSolutionFoundSoFar` across all these tested lengths.
*   The final result presented to the user is this overall best. This means the optimal path might use fewer tiles than the user's specified maximum if a shorter path yields a higher profit.

## Application Flow & UI (`main.go` & `ui.go` with Ebitengine)

The application uses Ebitengine for its graphical user interface and manages its flow through different states. UI elements are handled in `ui.go`, while the main application loop and state management reside in `main.go`.

### Game States

*   `StatePlacingRoad`: User draws roads.
*   `StatePlacingRiverSource`: User selects the river's starting point.
*   `StateCalculating`: The application is actively searching for the optimal solution.
*   `StateShowingResult`: The best found solution is displayed.

### UI Panel

A side panel provides controls and information:
*   **Status Display**: Shows current game state, profit scores, path lengths, calculation progress, and selected river length.
*   **Buttons**: Dynamically change based on the current game state.
*   **River Length Adjustment**:
    *   A **scrollbar** allows fine-grained control of the `currentMaxRiverLength`.
    *   **PageUp/PageDown keys** also adjust this value.

### Key Controls and Button Functions

**Common:**
*   **PageUp/PageDown**: Adjust `currentMaxRiverLength` (used for the next calculation).
*   **Reset All (Clear Map)**: Stops any ongoing calculation and resets the application to the initial `StatePlacingRoad`, clearing all roads, river, and forest tiles.

**State: `StatePlacingRoad`**
*   **Left Mouse Button (on grid)**: Places a `Road` tile.
*   **Right Mouse Button (on grid)**: Deletes a `Road` tile.
*   **"Cross Adj: ON/OFF" Button**: Toggles the `DisableCrossRiverAdjacency` rule for the river pathfinding.
*   **"Finalize Road & Select Source" Button**:
    *   Saves the current road layout.
    *   Transitions to `StatePlacingRiverSource`.
    *   Identifies and highlights valid river starting positions on the border.

**State: `StatePlacingRiverSource`**
*   **Left Mouse Button (on highlighted border tile)**: Selects that tile as the river source.
*   **"Cross Adj: ON/OFF" Button**: Toggles the `DisableCrossRiverAdjacency` rule.
*   **"Start Calculation" Button**:
    *   Becomes active once a valid river source is selected.
    *   Transitions to `StateCalculating`.
    *   Launches a goroutine to perform the iterative river length calculation.
*   **"Edit Road Layout" Button**: Returns to `StatePlacingRoad`.
*   **Escape Key**: Returns to `StatePlacingRoad`, clearing any selected river source.

**State: `StateCalculating`**
*   The UI displays the grid and path of the `overallBestSolutionInIterativeRun` as it's being updated.
*   The status message shows:
    *   The current river length being tested out of the target maximum.
    *   The best profit found for the current length being tested.
    *   The overall best profit and path length found across all tested lengths so far.
    *   Elapsed time for the calculation.
*   **"Stop Calculation" Button**:
    *   Stops the calculation goroutine.
    *   Transitions to `StateShowingResult`, displaying the best solution found up to the point of stopping.
*   **Escape Key**: Same as "Stop Calculation" button.

**State: `StateShowingResult`**
*   Displays the `finalBestSolution.Grid` (which is the `overallBestSolutionInIterativeRun` from the calculation).
*   Status message shows: final profit, actual path length of the best solution, and the maximum river length that was used to find this best solution.
*   **"Recalculate (New Max Len)" Button**:
    *   Transitions back to `StateCalculating`.
    *   Starts a new iterative calculation using the current `currentMaxRiverLength` (which might have been adjusted by the user while viewing results) and the previously used river start.
*   **"Change River Start" Button**:
    *   Transitions to `StatePlacingRiverSource`.
    *   The existing road layout is preserved. The user can select a new river starting point.
*   **"Edit Road Layout" Button**:
    *   Transitions to `StatePlacingRoad`.
    *   The existing road layout is loaded for editing.
*   **Escape Key**: Transitions to `StatePlacingRiverSource` to allow changing the river start point with the current road layout.

### Concurrency

*   A `sync.Mutex` (`g.mu`) is used to protect shared game state that might be accessed by the main Ebitengine loop and the calculation goroutine.
*   A `stopCalcChannel` is used to signal the calculation goroutine to terminate prematurely if the user stops the calculation or resets the game.

## Development Notes

The river pathfinding heuristics were a key area of iterative development:
1.  Initial attempts involved simple exhaustive search.
2.  Various border preference strategies were tested (e.g., preferring non-border tiles for a certain depth or universally).
3.  Straightness preference was introduced.
4.  The concept of an "Adjacency Bonus" (how many existing river segments a potential forest spot would be next to) was added and combined with straightness preference in different orders.
5.  The "New Forest Tiles Count" (number of new empty neighbors for forest placement) was added as another scoring criterion.
6.  The final refined heuristic prioritizes Adjacency Bonus, then New Forest Tiles Count, and then prefers turns over straight lines if the prior scores are tied. This aims to maximize the density of forests around river segments.

The UI was also refactored, moving panel drawing logic and UI element definitions into a separate `ui.go` file for better code organization.

## Game Rules

- The game is played on a 12-tile high and 21-tile wide grid.
- A circular road is present on the map. The user will define the road tiles within the application.
- No tiles (River or Forest) can be placed within one tile of the road (including diagonals).
- **River Tiles**:
    - The river source must be placed on a map border tile.
    - Each subsequent river tile must be placed adjacent (not diagonally) to the last placed river tile.
    - River tiles double the effect of adjacent (not diagonally) Forest tiles.
- **Forest Tiles**:
    - Each Forest tile placed gives a 2% attack speed bonus to the player.
    - If a Forest tile is adjacent to one or more River tiles, its attack speed bonus is doubled (to 4% per adjacent river segment, though the problem implies a single doubling if near *any* river part).

## Application Functionality

1.  **Road Definition**: The user can graphically draw the road path on the grid within the application.
2.  **Forbidden Zones**: The application will automatically mark tiles adjacent to the road as forbidden for placement.
3.  **Optimal Path Calculation**: The core feature is to calculate the best path for the river and the optimal placement of forest tiles around it to achieve the maximum possible attack speed bonus.
4.  **Visualization**: The application will display the grid, the road, forbidden zones, the calculated river path, and forest placements.

## How it Works (Planned)

The application will likely use a search algorithm (e.g., recursive backtracking or a variation of pathfinding) to:
1.  Identify all possible valid starting positions for the river (border tiles not blocked by the road or its forbidden zone).
2.  Explore all possible valid river paths from these starting positions.
3.  For each valid river path, determine the optimal placement of Forest tiles in the remaining available and valid spots.
4.  Calculate the total attack speed bonus for each configuration (River path + Forest placements).
5.  Identify and display the configuration that yields the maximum profit.

## Technologies

-   **Language**: Go
-   **Graphics/UI**: Ebitengine (a 2D game engine for Go) 