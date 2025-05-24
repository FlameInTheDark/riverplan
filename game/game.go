package game

import (
	"fmt"
	"sort"
	// "math/rand" // No longer needed for deterministic search
)

// Grid dimensions
const (
	GridHeight = 12
	GridWidth  = 21
)

// TileType represents the type of a tile on the game grid.
// We'll use iota to give these constants incrementing values.
const (
	Empty     TileType = iota // Default state, can be built upon
	Road                      // Manually placed road tile
	River                     // Player-placed river tile
	Forest                    // Player-placed forest tile
	Forbidden                 // Tiles near the road or otherwise unbuildable
)

// TileType is an alias for int for better readability.
type TileType int

// Coordinate represents a position on the grid.
// X is column, Y is row.
type Coordinate struct {
	X, Y int
}

// Grid represents the game board.
// It's a 2D slice of TileType.
// The first dimension is Y (rows), second is X (columns).
type Grid [GridHeight][GridWidth]TileType

// NewGrid creates and returns an initialized game grid.
// All tiles are set to Empty by default.
func NewGrid() Grid {
	var grid Grid
	// Go initializes arrays with their zero value, which for TileType (int) is 0 (Empty).
	// So, the grid is already initialized to Empty.
	// If we wanted a different default, we would iterate here.
	return grid
}

// SetRoad places road tiles on the grid and marks adjacent tiles as Forbidden.
func (g *Grid) SetRoad(roadTiles []Coordinate) {
	// First, clear all existing Road and Forbidden tiles to handle removals correctly
	// and ensure a clean slate for re-applying road and new forbidden zones.
	for y := 0; y < GridHeight; y++ {
		for x := 0; x < GridWidth; x++ {
			if g[y][x] == Road || g[y][x] == Forbidden {
				g[y][x] = Empty
			}
		}
	}

	// Place new road tiles
	for _, roadTile := range roadTiles {
		if g.isValidCoordinate(roadTile) {
			g[roadTile.Y][roadTile.X] = Road
		}
	}

	// Mark adjacent (non-diagonal) tiles to new road as Forbidden
	for _, roadTile := range roadTiles {
		neighbors := []Coordinate{
			{X: roadTile.X, Y: roadTile.Y - 1}, // Up
			{X: roadTile.X, Y: roadTile.Y + 1}, // Down
			{X: roadTile.X - 1, Y: roadTile.Y}, // Left
			{X: roadTile.X + 1, Y: roadTile.Y}, // Right
		}

		for _, adjCoord := range neighbors {
			if g.isValidCoordinate(adjCoord) && g[adjCoord.Y][adjCoord.X] == Empty {
				g[adjCoord.Y][adjCoord.X] = Forbidden
			}
		}
	}
}

// isValidCoordinate checks if a coordinate is within the grid boundaries.
func (g *Grid) isValidCoordinate(c Coordinate) bool {
	return c.X >= 0 && c.X < GridWidth && c.Y >= 0 && c.Y < GridHeight
}

// Print displays the current state of the grid to the console.
func (g *Grid) Print() {
	for y := 0; y < GridHeight; y++ {
		for x := 0; x < GridWidth; x++ {
			switch g[y][x] {
			case Empty:
				fmt.Print(". ") // Dot for Empty
			case Road:
				fmt.Print("R ") // R for Road
			case River:
				fmt.Print("~ ") // ~ for River
			case Forest:
				fmt.Print("F ") // F for Forest
			case Forbidden:
				fmt.Print("X ") // X for Forbidden
			default:
				fmt.Print("? ") // Should not happen
			}
		}
		fmt.Println()
	}
}

// GetValidRiverStarts identifies all valid starting positions for a river.
// A river can only start on a border tile that is currently Empty and not a corner.
func (g *Grid) GetValidRiverStarts() []Coordinate {
	var validStarts []Coordinate

	// Check top and bottom borders (excluding corners)
	for x := 1; x < GridWidth-1; x++ { // Start from x=1 and end before GridWidth-1
		// Top border
		if g[0][x] == Empty {
			validStarts = append(validStarts, Coordinate{X: x, Y: 0})
		}
		// Bottom border
		if g[GridHeight-1][x] == Empty {
			validStarts = append(validStarts, Coordinate{X: x, Y: GridHeight - 1})
		}
	}

	// Check left and right borders (excluding corners)
	for y := 1; y < GridHeight-1; y++ { // Start from y=1 and end before GridHeight-1
		// Left border
		if g[y][0] == Empty {
			validStarts = append(validStarts, Coordinate{X: 0, Y: y})
		}
		// Right border
		if g[y][GridWidth-1] == Empty {
			validStarts = append(validStarts, Coordinate{X: GridWidth - 1, Y: y})
		}
	}
	return validStarts
}

// RiverPathSolution stores a sequence of river tiles and the calculated profit.
type RiverPathSolution struct {
	Path   []Coordinate
	Profit float64
	Grid   Grid
}

// FindOptimalRiverAndForests now accepts maxLen and disableCrossRiverAdjacency.
func (g *Grid) FindOptimalRiverAndForests(startCoordinate Coordinate, maxLen int, progressCallback func(RiverPathSolution), stopChannel <-chan struct{}, disableCrossRiverAdjacency bool) (RiverPathSolution, error) {
	fmt.Printf("Starting search from user-defined start: (%d, %d) with max length: %d, DisableCrossAdj: %t\n", startCoordinate.X, startCoordinate.Y, maxLen, disableCrossRiverAdjacency)
	initialGrid := *g

	bestSolution := RiverPathSolution{Profit: -1.0, Grid: initialGrid}
	if initialGrid[startCoordinate.Y][startCoordinate.X] != Empty {
		return bestSolution, fmt.Errorf("chosen river start point (%d, %d) is not Empty", startCoordinate.X, startCoordinate.Y)
	}
	var currentPath []Coordinate
	workingGrid := initialGrid

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in FindOptimalRiverAndForests (likely from closed stopChannel):", r)
		}
	}()
	exploreAndEvaluateRecursive(&workingGrid, startCoordinate, currentPath, &bestSolution, 0, maxLen, progressCallback, stopChannel, disableCrossRiverAdjacency)

	select {
	case <-stopChannel:
		fmt.Println("Search was stopped prematurely via channel.")
		return bestSolution, fmt.Errorf("search stopped by user")
	default:
	}

	if bestSolution.Profit < 0 {
		return RiverPathSolution{Grid: *g, Profit: -1.0}, fmt.Errorf("no profitable river paths found from (%d, %d) with max length %d", startCoordinate.X, startCoordinate.Y, maxLen)
	}
	fmt.Printf("Search complete. Best profit: %.2f%% with %d river tiles from start (%d, %d), max length %d.\n", bestSolution.Profit*100, len(bestSolution.Path), startCoordinate.X, startCoordinate.Y, maxLen)
	return bestSolution, nil
}

// exploreAndEvaluateRecursive now uses maxLen and disableCrossRiverAdjacency.
func exploreAndEvaluateRecursive(grid *Grid, currentTile Coordinate, currentPath []Coordinate, bestSolution *RiverPathSolution, depth int, maxLen int, progressCallback func(RiverPathSolution), stopChannel <-chan struct{}, disableCrossRiverAdjacency bool) {
	select {
	case <-stopChannel:
		return
	default:
	}

	// depth is 0-indexed count of tiles being placed. If depth == maxLen, we've placed maxLen tiles already (0 to maxLen-1).
	// So, currentTile would be the (maxLen+1)th tile, which is too much.
	if depth >= maxLen { // Correct check: if depth (0-indexed) is maxLen, path would become maxLen+1
		return
	}

	originalTileState := grid[currentTile.Y][currentTile.X]
	if originalTileState != Empty {
		return
	}
	grid[currentTile.Y][currentTile.X] = River
	pathWithCurrentTile := append(currentPath, currentTile)

	madeRecursiveCall := false
	if len(pathWithCurrentTile) < maxLen {
		potentialNeighbors := []Coordinate{
			{X: currentTile.X, Y: currentTile.Y - 1}, // Up
			{X: currentTile.X, Y: currentTile.Y + 1}, // Down
			{X: currentTile.X - 1, Y: currentTile.Y}, // Left
			{X: currentTile.X + 1, Y: currentTile.Y}, // Right
		}

		nonBorderChoices := []Coordinate{}
		borderChoices := []Coordinate{}

		for _, nextTile := range potentialNeighbors {
			// Stop channel check inside loop
			select {
			case <-stopChannel:
				grid[currentTile.Y][currentTile.X] = originalTileState
				return
			default:
			}

			// U-turn prevention
			if len(pathWithCurrentTile) >= 2 {
				grandParentTile := pathWithCurrentTile[len(pathWithCurrentTile)-2]
				if nextTile.X == grandParentTile.X && nextTile.Y == grandParentTile.Y {
					continue
				}
			}

			// Cross Adjacency Check (if enabled)
			if disableCrossRiverAdjacency {
				isCrossAdjacent := false
				potentialCrossAdjacents := []Coordinate{
					{X: nextTile.X, Y: nextTile.Y - 1}, {X: nextTile.X, Y: nextTile.Y + 1},
					{X: nextTile.X - 1, Y: nextTile.Y}, {X: nextTile.X + 1, Y: nextTile.Y},
				}
				for _, adjToNext := range potentialCrossAdjacents {
					if adjToNext.X == currentTile.X && adjToNext.Y == currentTile.Y {
						continue
					}
					if grid.isValidCoordinate(adjToNext) && grid[adjToNext.Y][adjToNext.X] == River {
						isCrossAdjacent = true
						break
					}
				}
				if isCrossAdjacent {
					continue
				}
			}

			if grid.isValidCoordinate(nextTile) && grid[nextTile.Y][nextTile.X] == Empty {
				isBorder := nextTile.X == 0 || nextTile.X == GridWidth-1 || nextTile.Y == 0 || nextTile.Y == GridHeight-1
				if isBorder {
					borderChoices = append(borderChoices, nextTile)
				} else {
					nonBorderChoices = append(nonBorderChoices, nextTile)
				}
			}
		}

		var currentConsiderationSet []Coordinate
		if len(nonBorderChoices) > 0 {
			currentConsiderationSet = nonBorderChoices
		} else if len(borderChoices) > 0 {
			currentConsiderationSet = borderChoices
		} // If both are empty, madeRecursiveCall remains false, path terminates.

		if len(currentConsiderationSet) > 0 {
			// Score and sort moves
			scoredMoves := make([]ScoredMove, 0, len(currentConsiderationSet))

			var dxPrev, dyPrev int
			hasPrevDirection := false
			if len(pathWithCurrentTile) >= 2 {
				parentTile := pathWithCurrentTile[len(pathWithCurrentTile)-2]
				dxPrev = currentTile.X - parentTile.X
				dyPrev = currentTile.Y - parentTile.Y
				hasPrevDirection = true
			}

			for _, choice := range currentConsiderationSet {
				isStraight := false
				if hasPrevDirection {
					dxNext := choice.X - currentTile.X
					dyNext := choice.Y - currentTile.Y
					if dxNext == dxPrev && dyNext == dyPrev {
						isStraight = true
					}
				}

				adjacencyBonus := 0
				// Calculate adjacency bonus for this 'choice'
				// Potential forest spots are neighbors of 'choice' that are 'Empty'
				choiceNeighbors := []Coordinate{
					{X: choice.X, Y: choice.Y - 1}, {X: choice.X, Y: choice.Y + 1},
					{X: choice.X - 1, Y: choice.Y}, {X: choice.X + 1, Y: choice.Y},
				}
				for _, pForest := range choiceNeighbors {
					if grid.isValidCoordinate(pForest) && grid[pForest.Y][pForest.X] == Empty {
						// Now, count how many segments of pathWithCurrentTile (excluding 'choice' itself, but including currentTile)
						// this pForest is adjacent to.
						for _, riverSegInPath := range pathWithCurrentTile { // pathWithCurrentTile includes currentTile
							if (abs(pForest.X-riverSegInPath.X) == 1 && pForest.Y == riverSegInPath.Y) ||
								(abs(pForest.Y-riverSegInPath.Y) == 1 && pForest.X == riverSegInPath.X) {
								adjacencyBonus++
							}
						}
					}
				}

				newForestCount := 0
				for _, pForest := range choiceNeighbors { // Re-use choiceNeighbors for this count
					if grid.isValidCoordinate(pForest) && grid[pForest.Y][pForest.X] == Empty {
						newForestCount++
					}
				}

				scoredMoves = append(scoredMoves, ScoredMove{Coord: choice, IsStraight: isStraight, AdjacencyBonus: adjacencyBonus, NewForestTilesCount: newForestCount})
			}

			// Sort scoredMoves: Primary: AdjacencyBonus (desc), Secondary: NewForestTilesCount (desc), Tertiary: IsStraight (turns preferred)
			sort.Slice(scoredMoves, func(i, j int) bool {
				if scoredMoves[i].AdjacencyBonus != scoredMoves[j].AdjacencyBonus {
					return scoredMoves[i].AdjacencyBonus > scoredMoves[j].AdjacencyBonus // Higher bonus first
				}
				if scoredMoves[i].NewForestTilesCount != scoredMoves[j].NewForestTilesCount {
					return scoredMoves[i].NewForestTilesCount > scoredMoves[j].NewForestTilesCount // Higher count first
				}
				// If other scores are equal, prefer turns (IsStraight = false) over straights (IsStraight = true).
				// A turn (false) should come before a straight (true).
				if scoredMoves[i].IsStraight == false && scoredMoves[j].IsStraight == true { // i is Turn, j is Straight
					return true // i comes before j
				}
				if scoredMoves[i].IsStraight == true && scoredMoves[j].IsStraight == false { // i is Straight, j is Turn
					return false // j comes before i
				}
				return false // Both are same (both turns or both straights), order doesn't matter for this criterion
			})

			// Explore sorted moves
			for _, scoredChoice := range scoredMoves {
				exploreAndEvaluateRecursive(grid, scoredChoice.Coord, pathWithCurrentTile, bestSolution, depth+1, maxLen, progressCallback, stopChannel, disableCrossRiverAdjacency)
				madeRecursiveCall = true
			}
		}
		// If currentConsiderationSet was empty, madeRecursiveCall remains false, and the path terminates naturally here.

	}

	select {
	case <-stopChannel:
		grid[currentTile.Y][currentTile.X] = originalTileState
		return
	default:
	}

	if !madeRecursiveCall || len(pathWithCurrentTile) == maxLen { // Evaluate if path ends naturally or hits maxLen
		profit, gridWithForests := calculateProfitAndPlaceForests(*grid, pathWithCurrentTile)
		if profit > bestSolution.Profit {
			select {
			case <-stopChannel:
				grid[currentTile.Y][currentTile.X] = originalTileState
				return
			default:
				bestSolution.Profit = profit
				bestSolution.Path = make([]Coordinate, len(pathWithCurrentTile))
				copy(bestSolution.Path, pathWithCurrentTile)
				bestSolution.Grid = gridWithForests
				if progressCallback != nil {
					progressCallback(*bestSolution)
				}
			}
		}
	}

	grid[currentTile.Y][currentTile.X] = originalTileState
}

// Helper function for absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// calculateProfitAndPlaceForests places Forest tiles ONLY in Empty spots adjacent to the river,
// and calculates profit where each adjacent river tile DOUBLES the forest's base 2% profit.
func calculateProfitAndPlaceForests(gridWithRiver Grid, riverPath []Coordinate) (float64, Grid) {
	workingGrid := gridWithRiver // Start with the grid that has the river placed

	potentialForestSpots := make(map[Coordinate]bool)
	for _, riverTile := range riverPath {
		adjacents := []Coordinate{
			{X: riverTile.X, Y: riverTile.Y - 1}, {X: riverTile.X, Y: riverTile.Y + 1},
			{X: riverTile.X - 1, Y: riverTile.Y}, {X: riverTile.X + 1, Y: riverTile.Y},
		}
		for _, adj := range adjacents {
			if workingGrid.isValidCoordinate(adj) && workingGrid[adj.Y][adj.X] == Empty {
				potentialForestSpots[adj] = true
			}
		}
	}

	for spot := range potentialForestSpots {
		workingGrid[spot.Y][spot.X] = Forest
	}

	totalProfit := 0.0
	for y := 0; y < GridHeight; y++ {
		for x := 0; x < GridWidth; x++ {
			if workingGrid[y][x] == Forest {
				baseForestProfit := 0.02 // Base 2% profit
				adjacentRiverCount := 0

				adjacentsToForest := []Coordinate{
					{X: x, Y: y - 1}, {X: x, Y: y + 1},
					{X: x - 1, Y: y}, {X: x + 1, Y: y},
				}

				for _, adj := range adjacentsToForest {
					if workingGrid.isValidCoordinate(adj) && workingGrid[adj.Y][adj.X] == River {
						adjacentRiverCount++
					}
				}

				// New profit calculation logic:
				// FinalProfit = BaseForestProfit * (2 * NumberOfAdjacentRiverTiles)
				individualForestProfit := 0.0
				if adjacentRiverCount > 0 {
					individualForestProfit = baseForestProfit * (2.0 * float64(adjacentRiverCount))
				}
				// If a forest tile somehow has 0 adjacent rivers (which shouldn't happen
				// with current placement logic), its profit contribution here will be 0.0.

				totalProfit += individualForestProfit
			}
		}
	}
	return totalProfit, workingGrid
}

// TODO: Add functions for calculating profit based on a river path and forest placements

type ScoredMove struct {
	Coord               Coordinate
	IsStraight          bool
	AdjacencyBonus      int
	NewForestTilesCount int
}
