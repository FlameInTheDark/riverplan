# River Plan Optimizer

This application helps plan optimal placement of River and Forest tiles in a game to maximize a player's attack speed bonus.

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