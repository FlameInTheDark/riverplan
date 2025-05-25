package main

import (
	"bytes" // Needed for bytes.NewReader with the new clipboard library
	"fmt"
	"image"
	"image/color" // Needed for decoding PNG from clipboard
	"log"
	"os"
	"riverplan/game"
	"sync"
	"time"

	// "github.com/atotto/clipboard" // Removing this library
	"golang.design/x/clipboard" // Using this library instead for image support

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/sqweek/dialog"
)

const (
	gameAreaWidth             = game.GridWidth * tileSize
	screenWidth               = gameAreaWidth + panelWidth // Total window width
	screenHeight              = game.GridHeight * tileSize
	tileSize                  = 32 // Size of each tile in pixels
	minRiverLength            = 5
	maxRiverLengthCap         = 35 // Absolute cap for slider adjustment (CHANGED FROM 100 to 35)
	defaultInitialRiverLength = 35
	// brightnessDifferenceThreshold is the amount by which a tile's brightness must exceed the
	// reference tile's brightness to be considered a road.
	brightnessDifferenceThreshold = 15.0

	// Define the target road color (Loop Hero roads are brownish-yellow)
	// This might need adjustment. Let's start with a sample color.
	// Values are R, G, B (0-255).
	// roadColorR = 110 // Adjusted based on user screenshot - NO LONGER USED
	// roadColorG = 110 // Adjusted based on user screenshot - NO LONGER USED
	// roadColorB = 110 // Adjusted based on user screenshot - NO LONGER USED
	// Threshold for color matching (sum of absolute differences in R, G, B values)
	// colorThreshold = 70 // Adjusted: Made stricter to avoid non-road tiles like campfire - NO LONGER USED

	// Anchor colors for grid detection
	anchorDarkGrayR      = 58  // Reverted to user-specified #3A3F3F for top-left
	anchorDarkGrayG      = 63  // Reverted to user-specified #3A3F3F for top-left
	anchorDarkGrayB      = 63  // Reverted to user-specified #3A3F3F for top-left
	anchorGreenPatternR  = 142 // User-specified #8E8F7B for top-right
	anchorGreenPatternG  = 143 // User-specified #8E8F7B for top-right
	anchorGreenPatternB  = 123 // User-specified #8E8F7B for top-right
	anchorColorThreshold = 30  // Threshold for matching anchor colors (can be tuned)

	// Percentage-based grid coordinates (derived from 2560x1440 example)
	// Top-left X of grid = 0px -> 0%
	// Top-left Y of grid = 100px -> 100/1440 = 6.944%
	// Top-right X of grid = 2099px -> 2099/2560 = 81.99%
	// Grid bottom Y: TopY_px + ( (TopRightX_px - TopLeftX_px)/21 tiles * 12 tiles ) = 100 + ( (2099-0)/21 * 12 ) = 100 + 1199.428 = 1299.428px
	//                 1299.428px / 1440px height = 90.2379...%
	gridStartXPercent  = 0.0
	gridStartYPercent  = 0.0694444444
	gridEndXPercent    = 0.819921875
	gridBottomYPercent = 0.9023791667

	// UI Button constants
	// panelWidth, buttonHeight, buttonMargin, buttonPadding, textOffsetY moved to ui.go
)

// GameState defines the current state of the game interaction.
type GameState int

const (
	StatePlacingRoad GameState = iota
	StatePlacingRiverSource
	StateCalculating
	StateShowingResult
)

// Button struct for UI elements - moved to ui.go

// Game implements ebiten.Game interface.
type Game struct {
	grid                            game.Grid // Current working grid, might show intermediate or final results
	roadLayoutGrid                  game.Grid // Stores the grid with only roads and forbidden tiles
	gameState                       GameState
	calculationStatus               string
	finalBestSolution               game.RiverPathSolution
	intermediateBestSolution        game.RiverPathSolution // Overall best intermediate solution
	selectedRiverStart              game.Coordinate
	validRiverStarts                []game.Coordinate // To highlight valid spots for user
	calculationStartTime            time.Time
	stopCalcChannel                 chan struct{} // Single channel to stop the calculation goroutine
	currentMaxRiverLength           int           // User-adjustable, potentially for next calculation
	lengthUsedForCurrentCalculation int           // New: Stores the max length the current calculation was started with
	maxLenUsedForFinalSolution      int           // Max length used to get the g.finalBestSolution
	DisableCrossRiverAdjacency      bool          // New: Toggle for cross-river adjacency rule
	mu                              sync.Mutex

	// Fields for iterative calculation state management
	isIterativeCalculationActive      bool
	currentLengthBeingTested          int
	overallBestSolutionInIterativeRun game.RiverPathSolution

	// UI elements - can be dynamic based on state
	buttons []Button

	// Rects for custom UI controls like river length adjuster
	minusRiverLengthButtonRect image.Rectangle // Will be removed or repurposed
	plusRiverLengthButtonRect  image.Rectangle // Will be removed or repurposed

	// Scrollbar specific fields
	scrollBarRect       image.Rectangle
	scrollThumbRect     image.Rectangle
	isDraggingScrollBar bool
	dragOffsetX         int // To maintain relative drag position on the thumb
}

// NewGame initializes a new game instance.
func NewGame() *Game {
	g := &Game{
		grid:                              game.NewGrid(),
		roadLayoutGrid:                    game.NewGrid(), // Initially empty
		gameState:                         StatePlacingRoad,
		currentMaxRiverLength:             defaultInitialRiverLength,
		lengthUsedForCurrentCalculation:   defaultInitialRiverLength, // Initialize
		maxLenUsedForFinalSolution:        0,                         // No solution yet
		DisableCrossRiverAdjacency:        false,                     // Default for the new toggle
		isIterativeCalculationActive:      false,
		currentLengthBeingTested:          0,
		overallBestSolutionInIterativeRun: game.RiverPathSolution{Profit: -1.0},
	}
	// Initialize solutions with the empty grid state
	g.finalBestSolution.Grid = g.grid
	g.finalBestSolution.Profit = -1.0
	g.intermediateBestSolution.Grid = g.grid
	g.intermediateBestSolution.Profit = -1.0
	g.overallBestSolutionInIterativeRun.Grid = g.grid
	g.updateButtonsForState()   // Initialize buttons
	g.updateCalculationStatus() // Initialize status
	return g
}

// Helper to update calculationStatus string based on current state and data
func (g *Game) updateCalculationStatus() {
	switch g.gameState {
	case StatePlacingRoad:
		g.calculationStatus = fmt.Sprintf("Max Len: %d (PgUp/PgDn: 5-%d)", g.currentMaxRiverLength, maxRiverLengthCap)
	case StatePlacingRiverSource:
		statusText := fmt.Sprintf("Max Len: %d (PgUp/PgDn: 5-%d).", g.currentMaxRiverLength, maxRiverLengthCap)
		if g.selectedRiverStart.X != 0 || g.selectedRiverStart.Y != 0 { // Check if a start is selected (assuming (0,0) is not a valid start)
			statusText += fmt.Sprintf("\nSelected Start: (%d, %d)", g.selectedRiverStart.X, g.selectedRiverStart.Y)
		} else {
			statusText += "\nClick valid border tile for river source."
		}
		g.calculationStatus = statusText
	case StateCalculating:
		if g.isIterativeCalculationActive {
			status := fmt.Sprintf("Iterative Calc (Max %d):\n", g.lengthUsedForCurrentCalculation)
			status += fmt.Sprintf("Testing Len: %d / %d\n", g.currentLengthBeingTested, g.lengthUsedForCurrentCalculation)
			profitCurrentTest := 0.0
			if g.intermediateBestSolution.Profit >= 0 {
				profitCurrentTest = g.intermediateBestSolution.Profit * 100
			}
			status += fmt.Sprintf("Cur Best: %.2f%% (Path %d)\n", profitCurrentTest, len(g.intermediateBestSolution.Path))
			profitOverall := 0.0
			if g.overallBestSolutionInIterativeRun.Profit >= 0 {
				profitOverall = g.overallBestSolutionInIterativeRun.Profit * 100
			}
			status += fmt.Sprintf("Overall Best: %.2f%% (Path %d)\n", profitOverall, len(g.overallBestSolutionInIterativeRun.Path))
			status += fmt.Sprintf("(%.1fs)", time.Since(g.calculationStartTime).Seconds())
			g.calculationStatus = status
		} else { // Original non-iterative status, can be kept as fallback or removed if iterative is always used
			profit := g.intermediateBestSolution.Profit
			if profit < 0 {
				profit = 0
			}
			g.calculationStatus = fmt.Sprintf("Calculating (MaxLen %d)...\nBest: %.2f%% (Path: %d)\n(%.1fs).",
				g.lengthUsedForCurrentCalculation,
				profit*100,
				len(g.intermediateBestSolution.Path),
				time.Since(g.calculationStartTime).Seconds())
		}
	case StateShowingResult:
		profit := g.finalBestSolution.Profit
		if profit < 0 {
			profit = 0
		}
		status := fmt.Sprintf("Result Profit: %.2f%%\n(Path: %d, Used MaxLen: %d). ",
			profit*100,
			len(g.finalBestSolution.Path),
			g.maxLenUsedForFinalSolution)
		status += fmt.Sprintf("\nAdj. MaxLen: %d (PgUp/PgDn: 5-%d).", g.currentMaxRiverLength, maxRiverLengthCap)
		g.calculationStatus = status
	}
}

// Update proceeds the game state.
// Update is called every tick (1/60 [s] by default).
func (g *Game) Update() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Handle river length adjustment (can be done in most states)
	// We check for IsKeyJustPressed to only increment once per press
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		if g.currentMaxRiverLength < maxRiverLengthCap {
			g.currentMaxRiverLength++
			g.updateCalculationStatus()
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		if g.currentMaxRiverLength > minRiverLength {
			g.currentMaxRiverLength--
			g.updateCalculationStatus()
		}
	}

	// Handle mouse clicks for buttons or grid
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mouseX, mouseY := ebiten.CursorPosition()
		clickedPoint := image.Point{X: mouseX, Y: mouseY}

		// Check UI buttons first
		panelClicked := false
		for _, button := range g.buttons {
			if clickedPoint.In(button.Rect) {
				button.OnClick(g)
				panelClicked = true
				break
			}
		}

		// Scrollbar interaction (if no main button was clicked)
		if !panelClicked {
			if clickedPoint.In(g.scrollThumbRect) {
				g.isDraggingScrollBar = true
				g.dragOffsetX = mouseX - g.scrollThumbRect.Min.X // Capture offset of click within thumb
				panelClicked = true                              // Consumed click for scrollbar drag
			} else if clickedPoint.In(g.scrollBarRect) { // Click on track, not thumb
				// Jump thumb to click position
				newThumbMinX := mouseX - (g.scrollThumbRect.Dx() / 2) // Center thumb on click
				// Clamp within scrollBarRect bounds
				if newThumbMinX < g.scrollBarRect.Min.X {
					newThumbMinX = g.scrollBarRect.Min.X
				}
				if newThumbMinX+g.scrollThumbRect.Dx() > g.scrollBarRect.Max.X {
					newThumbMinX = g.scrollBarRect.Max.X - g.scrollThumbRect.Dx()
				}

				trackWidthForThumb := g.scrollBarRect.Dx() - g.scrollThumbRect.Dx()
				if trackWidthForThumb <= 0 {
					trackWidthForThumb = 1
				} // Avoid div by zero

				percentage := float64(newThumbMinX-g.scrollBarRect.Min.X) / float64(trackWidthForThumb)
				newValue := minRiverLength + int(percentage*float64(maxRiverLengthCap-minRiverLength)+0.5) // +0.5 for rounding

				if newValue < minRiverLength {
					newValue = minRiverLength
				}
				if newValue > maxRiverLengthCap {
					newValue = maxRiverLengthCap
				}

				if g.currentMaxRiverLength != newValue {
					g.currentMaxRiverLength = newValue
					g.updateCalculationStatus()
					// updatePanelControlRects() will be called next frame, or call explicitly if immediate feedback needed for thumb
				}
				panelClicked = true // Consumed click for scrollbar track jump
			}
		}

		if !panelClicked && mouseX >= panelWidth { // Click is in game area
			gridX, gridY := (mouseX-panelWidth)/tileSize, mouseY/tileSize
			// Existing grid interaction logic based on gameState
			switch g.gameState {
			case StatePlacingRoad:
				if gridX >= 0 && gridX < game.GridWidth && gridY >= 0 && gridY < game.GridHeight {
					if g.grid[gridY][gridX] == game.Empty || g.grid[gridY][gridX] == game.Forbidden {
						var roadTiles []game.Coordinate
						for r := 0; r < game.GridHeight; r++ {
							for c := 0; c < game.GridWidth; c++ {
								if g.grid[r][c] == game.Road {
									roadTiles = append(roadTiles, game.Coordinate{X: c, Y: r})
								}
							}
						}
						roadTiles = append(roadTiles, game.Coordinate{X: gridX, Y: gridY})
						g.grid.SetRoad(roadTiles) // Modifies g.grid
						// No final/intermediate solution yet, ensure they reflect this empty/road-only state
						g.finalBestSolution.Grid = g.grid
						g.finalBestSolution.Profit = -1.0
						g.finalBestSolution.Path = nil
						g.intermediateBestSolution = g.finalBestSolution
					}
				}
			case StatePlacingRiverSource:
				clickedCoord := game.Coordinate{X: gridX, Y: gridY}
				isValidSource := false
				for _, validStart := range g.validRiverStarts {
					if validStart.X == clickedCoord.X && validStart.Y == clickedCoord.Y {
						isValidSource = true
						break
					}
				}
				fmt.Printf("[DEBUG] Grid click in StatePlacingRiverSource. Clicked: (%d,%d), IsValidSoFar: %t, NumValidStarts: %d\\n", clickedCoord.X, clickedCoord.Y, isValidSource, len(g.validRiverStarts))
				if isValidSource {
					g.selectedRiverStart = clickedCoord
					fmt.Printf("[DEBUG] River source selected by grid click: (%d, %d)\\\\n", g.selectedRiverStart.X, g.selectedRiverStart.Y)
					g.updateCalculationStatus() // Update status to show selected start, e.g., "Selected Start: (X,Y)"
					g.updateButtonsForState()   // Update buttons, e.g., "Start Calculation" button might become fully enabled or change text
				}
			}
		}
	}

	// Handle scrollbar dragging
	if g.isDraggingScrollBar {
		if ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
			mouseX, _ := ebiten.CursorPosition()
			newThumbMinX := mouseX - g.dragOffsetX // Apply original click offset

			// Clamp thumb position to within the scrollbar track
			if newThumbMinX < g.scrollBarRect.Min.X {
				newThumbMinX = g.scrollBarRect.Min.X
			}
			if newThumbMinX+g.scrollThumbRect.Dx() > g.scrollBarRect.Max.X {
				newThumbMinX = g.scrollBarRect.Max.X - g.scrollThumbRect.Dx()
			}

			// Convert thumb position back to currentMaxRiverLength value
			trackWidthForThumb := g.scrollBarRect.Dx() - g.scrollThumbRect.Dx()
			if trackWidthForThumb <= 0 { // Avoid division by zero if scrollbar is too small
				g.isDraggingScrollBar = false // Stop dragging if track is invalid
			} else {
				percentage := float64(newThumbMinX-g.scrollBarRect.Min.X) / float64(trackWidthForThumb)
				newValue := minRiverLength + int(percentage*float64(maxRiverLengthCap-minRiverLength)+0.5) // +0.5 for rounding

				if newValue < minRiverLength {
					newValue = minRiverLength
				}
				if newValue > maxRiverLengthCap {
					newValue = maxRiverLengthCap
				}

				if g.currentMaxRiverLength != newValue {
					g.currentMaxRiverLength = newValue
					g.updateCalculationStatus()
					// updatePanelControlRects() is called at the start of Update, so thumb will update visually
				}
			}
		} else { // Mouse button was released
			g.isDraggingScrollBar = false
		}
	}

	// RMB for deleting road tiles (if desired, keep separate from panel logic for now)
	if g.gameState == StatePlacingRoad && ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) && !inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mouseX, mouseY := ebiten.CursorPosition()
		if mouseX >= panelWidth { // Only if cursor is in game area
			gridX, gridY := (mouseX-panelWidth)/tileSize, mouseY/tileSize
			if gridX >= 0 && gridX < game.GridWidth && gridY >= 0 && gridY < game.GridHeight {
				if g.grid[gridY][gridX] == game.Road {
					var remainingRoadTiles []game.Coordinate
					for r := 0; r < game.GridHeight; r++ {
						for c := 0; c < game.GridWidth; c++ {
							if g.grid[r][c] == game.Road && !(c == gridX && r == gridY) {
								remainingRoadTiles = append(remainingRoadTiles, game.Coordinate{X: c, Y: r})
							}
						}
					}
					g.grid.SetRoad(remainingRoadTiles) // Modifies g.grid
					g.finalBestSolution.Grid = g.grid
					g.finalBestSolution.Profit = -1.0
					g.finalBestSolution.Path = nil
					g.intermediateBestSolution = g.finalBestSolution
				}
			}
		}
	}

	// Global Escape handling
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		switch g.gameState {
		case StatePlacingRiverSource:
			// Transition back to StatePlacingRoad
			g.gameState = StatePlacingRoad
			g.selectedRiverStart = game.Coordinate{} // Clear selected start
			g.validRiverStarts = nil
			// g.grid is already roadLayoutGrid or user is editing it
			fmt.Println("Escape pressed: Returning to Road Placement.")
			g.updateButtonsForState()
			g.updateCalculationStatus()
		case StateCalculating:
			// Stop calculation
			if g.stopCalcChannel != nil {
				close(g.stopCalcChannel)
				// The goroutine will handle state transition to StateShowingResult with intermediate results.
				fmt.Println("Escape pressed: Stop signal sent to calculation goroutine.")
				g.calculationStatus = "Stopping calculation..."
			}
		case StateShowingResult:
			// Transition to StatePlacingRiverSource
			g.gameState = StatePlacingRiverSource
			g.grid = g.roadLayoutGrid // Ensure grid shows road layout
			g.validRiverStarts = g.roadLayoutGrid.GetValidRiverStarts()
			g.intermediateBestSolution.Grid = g.roadLayoutGrid
			g.intermediateBestSolution.Path = nil
			g.intermediateBestSolution.Profit = -1.0
			g.finalBestSolution = g.intermediateBestSolution // Clear final solution as well
			g.maxLenUsedForFinalSolution = 0
			g.selectedRiverStart = game.Coordinate{} // Clear selected start, user needs to pick again
			fmt.Println("Escape pressed: Returning to River Source Selection.")
			g.updateButtonsForState()
			g.updateCalculationStatus()
		}
	}

	// Key-based controls (can be deprecated or kept as alternatives)
	// Example: R for Reset All (now also a button)
	if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		// g.resetButtonAction("Full") // This is now handled by button, or can be kept as a hotkey
	}

	return nil
}

// Draw draws the game screen.
// Draw is called every frame (typically 1/60 [s] for 60Hz display).
func (g *Game) Draw(screen *ebiten.Image) {
	// Debug print for button state at draw time
	tempButtonTexts := []string{}
	for _, btn := range g.buttons {
		tempButtonTexts = append(tempButtonTexts, btn.Text)
	}
	// Commenting out for cleaner logs unless specifically debugging button presence issues.
	// fmt.Printf("[DRAW DEBUG] State: %v, Buttons: %v\\n", g.gameState, tempButtonTexts)

	g.mu.Lock()
	defer g.mu.Unlock()

	// --- Draw Panel UI --- (MOVED to ui.go -> g.drawPanel())
	g.drawPanel(screen)

	// --- Draw Game Area ---
	gameImageOp := &ebiten.DrawImageOptions{}
	gameImageOp.GeoM.Translate(float64(panelWidth), 0) // panelWidth is a const from ui.go

	gameSubImage := ebiten.NewImage(gameAreaWidth, screenHeight)

	var drawGrid game.Grid
	switch g.gameState {
	case StatePlacingRoad:
		drawGrid = g.grid
	case StatePlacingRiverSource:
		drawGrid = g.roadLayoutGrid
	case StateCalculating:
		drawGrid = g.overallBestSolutionInIterativeRun.Grid
	case StateShowingResult:
		drawGrid = g.finalBestSolution.Grid
	default:
		drawGrid = g.grid
	}

	gameSubImage.Fill(color.RGBA{R: 50, G: 50, B: 50, A: 255})

	for y := 0; y < game.GridHeight; y++ {
		for x := 0; x < game.GridWidth; x++ {
			tileX, tileY := float64(x*tileSize), float64(y*tileSize)
			var tileColor color.Color

			currentTileType := drawGrid[y][x]

			// Highlight valid river starts in yellow if in that state, on top of the Empty tile color
			isHighlightedStart := false
			if g.gameState == StatePlacingRiverSource {
				for _, validStart := range g.validRiverStarts {
					if validStart.X == x && validStart.Y == y {
						isHighlightedStart = true
						break
					}
				}
			}

			if isHighlightedStart {
				tileColor = color.RGBA{R: 255, G: 255, B: 0, A: 255} // Bright Yellow for valid start
			} else {
				switch currentTileType {
				case game.Empty:
					tileColor = color.RGBA{R: 100, G: 100, B: 100, A: 255} // Gray
				case game.Road:
					tileColor = color.RGBA{R: 200, G: 200, B: 0, A: 255} // Yellowish for Road
				case game.River:
					tileColor = color.RGBA{R: 0, G: 0, B: 200, A: 255} // Blue
				case game.Forest:
					tileColor = color.RGBA{R: 0, G: 150, B: 0, A: 255} // Green
				case game.Forbidden:
					tileColor = color.RGBA{R: 150, G: 0, B: 0, A: 255} // Dark Red
				default:
					tileColor = color.RGBA{R: 30, G: 30, B: 30, A: 255} // Dark Gray for unknown
				}
			}
			ebitenutil.DrawRect(gameSubImage, tileX, tileY, float64(tileSize-1), float64(tileSize-1), tileColor)
		}
	}

	// Draw the current path from overallBestSolutionInIterativeRun if calculating iteratively
	if g.gameState == StateCalculating && g.isIterativeCalculationActive && len(g.overallBestSolutionInIterativeRun.Path) > 0 {
		pathColor := color.RGBA{R: 255, G: 105, B: 180, A: 200} // Hot pink
		if len(g.overallBestSolutionInIterativeRun.Path) > 0 {  // Redundant check, but safe
			firstTile := g.overallBestSolutionInIterativeRun.Path[0]
			ebitenutil.DrawRect(gameSubImage, float64(firstTile.X*tileSize), float64(firstTile.Y*tileSize), float64(tileSize-1), float64(tileSize-1), color.RGBA{R: 255, G: 0, B: 0, A: 100}) // Semi-transparent red overlay on start
		}
		for i := 0; i < len(g.overallBestSolutionInIterativeRun.Path)-1; i++ {
			p1 := g.overallBestSolutionInIterativeRun.Path[i]
			p2 := g.overallBestSolutionInIterativeRun.Path[i+1]
			x1 := float64(p1.X*tileSize) + float64(tileSize)/2
			y1 := float64(p1.Y*tileSize) + float64(tileSize)/2
			x2 := float64(p2.X*tileSize) + float64(tileSize)/2
			y2 := float64(p2.Y*tileSize) + float64(tileSize)/2
			ebitenutil.DrawLine(gameSubImage, x1, y1, x2, y2, pathColor)
		}
	} else if g.gameState == StateCalculating && !g.isIterativeCalculationActive && len(g.intermediateBestSolution.Path) > 0 {
		// Fallback for non-iterative calculation (if that path is ever re-enabled) or if isIterativeCalculationActive is somehow false
		pathColor := color.RGBA{R: 255, G: 105, B: 180, A: 200} // Hot pink
		if len(g.intermediateBestSolution.Path) > 0 {
			firstTile := g.intermediateBestSolution.Path[0]
			ebitenutil.DrawRect(gameSubImage, float64(firstTile.X*tileSize), float64(firstTile.Y*tileSize), float64(tileSize-1), float64(tileSize-1), color.RGBA{R: 255, G: 0, B: 0, A: 100})
		}
		for i := 0; i < len(g.intermediateBestSolution.Path)-1; i++ {
			p1 := g.intermediateBestSolution.Path[i]
			p2 := g.intermediateBestSolution.Path[i+1]
			x1 := float64(p1.X*tileSize) + float64(tileSize)/2
			y1 := float64(p1.Y*tileSize) + float64(tileSize)/2
			x2 := float64(p2.X*tileSize) + float64(tileSize)/2
			y2 := float64(p2.Y*tileSize) + float64(tileSize)/2
			ebitenutil.DrawLine(gameSubImage, x1, y1, x2, y2, pathColor)
		}
	} else if g.gameState == StateShowingResult && len(g.finalBestSolution.Path) > 0 {
		// Optionally, draw the final path distinctly if desired, or rely on grid colors
		// For now, grid colors (River tiles) show the path.
	}

	screen.DrawImage(gameSubImage, gameImageOp)

	// TPS/FPS counter at the bottom of the panel or screen -- This was part of drawPanel, ensure it's not duplicated or is placed globally if desired.
	// It was at the end of the panel drawing logic, so it's now in ui.go's drawPanel.
}

// Layout takes the outside size (e.g., window size) and returns the (logical) screen size.
// If you don't have to adjust the screen size with the outside size, just return a fixed size.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) updateButtonsForState() {
	g.buttons = []Button{}
	buttonMinX := buttonMargin
	buttonMaxX := panelWidth - buttonMargin

	switch g.gameState {
	case StatePlacingRoad:
		// Add Cross Adjacency Toggle Button for StatePlacingRoad
		crossAdjTextRoad := "Cross Adj: OFF"
		if g.DisableCrossRiverAdjacency {
			crossAdjTextRoad = "Cross Adj: ON"
		}
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: crossAdjTextRoad,
			OnClick: func(g *Game) {
				g.DisableCrossRiverAdjacency = !g.DisableCrossRiverAdjacency
				g.updateButtonsForState() // Refresh button panel
			},
		})
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Detect Road from Image File",
			OnClick: func(g *Game) {
				g.handleDetectRoadFromImage()
			},
		})
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Detect from Clipboard",
			OnClick: func(g *Game) {
				g.handleDetectRoadFromClipboard()
			},
		})
		// Ensure no trailing comma here before the next button or end of list
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Finalize Road & Select Source",
			OnClick: func(g *Game) {
				g.roadLayoutGrid = g.grid
				g.gameState = StatePlacingRiverSource
				g.validRiverStarts = g.roadLayoutGrid.GetValidRiverStarts()
				fmt.Printf("[DEBUG] Finalized Road. Number of valid river starts: %d. Starts: %v\\n", len(g.validRiverStarts), g.validRiverStarts)
				g.intermediateBestSolution.Grid = g.roadLayoutGrid
				g.intermediateBestSolution.Path = nil
				g.finalBestSolution = g.intermediateBestSolution
				g.selectedRiverStart = game.Coordinate{}
				fmt.Println("Road placement finalized. Stored roadLayoutGrid. Ready to select river source.")
				g.updateCalculationStatus()
				g.updateButtonsForState() // Ensure buttons refresh for the new state
			},
		})
	case StatePlacingRiverSource:
		// Add Cross Adjacency Toggle Button for StatePlacingRiverSource
		crossAdjTextSource := "Cross Adj: OFF"
		if g.DisableCrossRiverAdjacency {
			crossAdjTextSource = "Cross Adj: ON"
		}
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: crossAdjTextSource,
			OnClick: func(g *Game) {
				g.DisableCrossRiverAdjacency = !g.DisableCrossRiverAdjacency
				g.updateButtonsForState() // Refresh button panel
			},
		})

		isValidSrcSelected := false
		if !(g.selectedRiverStart.X == 0 && g.selectedRiverStart.Y == 0) {
			for _, vs := range g.validRiverStarts {
				if vs.X == g.selectedRiverStart.X && vs.Y == g.selectedRiverStart.Y {
					isValidSrcSelected = true
					break
				}
			}
		}
		startCalcButtonText := "Start Calculation"
		if !isValidSrcSelected {
			startCalcButtonText = "Start Calculation (Pick Source First)"
		}
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: startCalcButtonText,
			OnClick: func(g *Game) {
				fmt.Printf("[DEBUG] Start Calculation button clicked. Valid source selected: (%d, %d)\\n", g.selectedRiverStart.X, g.selectedRiverStart.Y)
				g.gameState = StateCalculating
				g.updateButtonsForState() // Ensure Stop button appears immediately
				g.calculationStartTime = time.Now()

				// Initialize for iterative calculation
				g.isIterativeCalculationActive = true
				g.overallBestSolutionInIterativeRun = game.RiverPathSolution{Grid: g.roadLayoutGrid, Profit: -1.0, Path: nil}
				g.intermediateBestSolution = g.overallBestSolutionInIterativeRun // Start with overall best as intermediate

				g.stopCalcChannel = make(chan struct{}) // Make sure this is fresh for each new calculation cycle
				fmt.Printf("[DEBUG] Set to StateCalculating. Target MaxLen %d. stopCalcChannel created: %p\n", g.currentMaxRiverLength, g.stopCalcChannel)

				gridForCalculation := g.roadLayoutGrid
				startNode := g.selectedRiverStart
				stopChan := g.stopCalcChannel // Capture for goroutine
				userSelectedMaxLength := g.currentMaxRiverLength
				disableCrossAdjacencyForCalc := g.DisableCrossRiverAdjacency

				g.lengthUsedForCurrentCalculation = userSelectedMaxLength // Store the user's target max length

				fmt.Printf("[DEBUG] Launching iterative calculation goroutine. Target MaxLen: %d, stopChan: %p, DisableCrossAdj: %t\n", userSelectedMaxLength, stopChan, disableCrossAdjacencyForCalc)

				go func() {
					fmt.Printf("[DEBUG] Iterative Goroutine started. Target MaxLen: %d, stopChan: %p\n", userSelectedMaxLength, stopChan)

					progressCb := func(intermediateSolutionForCurrentLength game.RiverPathSolution) {
						g.mu.Lock()
						// Update intermediate best for the *current length* being tested
						if intermediateSolutionForCurrentLength.Profit > g.intermediateBestSolution.Profit || g.intermediateBestSolution.Path == nil {
							g.intermediateBestSolution = intermediateSolutionForCurrentLength
						}

						// If this intermediate result is also better than the *overall best* found so far in this iterative run, update overall best
						if intermediateSolutionForCurrentLength.Profit > g.overallBestSolutionInIterativeRun.Profit {
							g.overallBestSolutionInIterativeRun = intermediateSolutionForCurrentLength
						}

						g.updateCalculationStatus()
						g.mu.Unlock()
					}

					// No local iterativeOverallBest needed; g.overallBestSolutionInIterativeRun is the source of truth

					for lengthToTest := minRiverLength; lengthToTest <= userSelectedMaxLength; lengthToTest++ {
						select {
						case <-stopChan:
							fmt.Println("[DEBUG] Iterative calc loop: stopChan closed before testing length", lengthToTest)
							goto endOfCalculation // Use goto to break out of nested loops and proceed to cleanup
						default:
						}

						g.mu.Lock()
						g.currentLengthBeingTested = lengthToTest
						// Reset intermediate best for this specific length test
						g.intermediateBestSolution = game.RiverPathSolution{Grid: gridForCalculation, Profit: -1.0, Path: nil}
						g.updateCalculationStatus() // Update status to show "Testing Len X..."
						g.mu.Unlock()

						fmt.Printf("[DEBUG] Iterative Goroutine: Testing length %d\n", lengthToTest)
						localGridCopy := gridForCalculation
						_, errThisLength := localGridCopy.FindOptimalRiverAndForests(startNode, lengthToTest, progressCb, stopChan, disableCrossAdjacencyForCalc)

						// After a length is fully tested (or stopped partway for this length)
						g.mu.Lock()
						if errThisLength == nil {
							// progressCb has already updated g.intermediateBestSolution with the best for this length,
							// and g.overallBestSolutionInIterativeRun if it was a new global best.
							// No specific action needed here for solutionForThisLength itself.
						} else if errThisLength.Error() == "search stopped by user" {
							// If search for this length was stopped, g.intermediateBestSolution holds the best for the partial run.
							// progressCb would have updated g.overallBestSolutionInIterativeRun if that partial result was a new global best.
							g.mu.Unlock()
							goto endOfCalculation // User stopped, break outer loop
						} else {
							fmt.Printf("[DEBUG] Error testing length %d: %v\n", lengthToTest, errThisLength)
						}
						g.updateCalculationStatus() // Update with potentially new overall best
						g.mu.Unlock()
					}

					// Label for goto to jump to the end of calculation processing
				endOfCalculation:
					g.mu.Lock()
					userForcedStop := false
					select {
					case <-stopChan: // Check if stopChan was closed
						userForcedStop = true
					default:
					}

					g.isIterativeCalculationActive = false // Iteration finished or stopped
					if g.stopCalcChannel == stopChan || (userForcedStop && g.stopCalcChannel == stopChan) {
						g.gameState = StateShowingResult
						if userForcedStop {
							fmt.Println("Iterative calculation stopped by user. Showing best overall result found.")
						} else { // Successful completion of all iterations
							fmt.Printf("Iterative Goroutine: All lengths tested. Overall Best Profit: %.2f%%. Path Len: %d\n", g.overallBestSolutionInIterativeRun.Profit*100, len(g.overallBestSolutionInIterativeRun.Path))
						}
						// g.overallBestSolutionInIterativeRun now holds the true overall best.
						g.finalBestSolution = g.overallBestSolutionInIterativeRun
						if g.finalBestSolution.Profit < 0 { // If nothing was ever found
							g.finalBestSolution.Grid = g.roadLayoutGrid
							g.finalBestSolution.Path = nil
						}
						g.maxLenUsedForFinalSolution = len(g.finalBestSolution.Path)
						if !userForcedStop { // Only update main display grid on natural completion
							g.grid = g.finalBestSolution.Grid
						}
						// Align intermediate with final for display in ShowingResult state
						g.intermediateBestSolution = g.finalBestSolution

						if g.stopCalcChannel == stopChan && !userForcedStop { // If calc completed naturally
							close(g.stopCalcChannel)
						}
						g.stopCalcChannel = nil // Clear the channel reference
					} else {
						fmt.Println("Goroutine for an older calculation finished or channel mismatch. No update to game state.")
					}

					g.updateButtonsForState()
					g.updateCalculationStatus()
					g.mu.Unlock()
				}()

			},
		})
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Edit Road Layout",
			OnClick: func(g *Game) {
				g.gameState = StatePlacingRoad
				g.grid = g.roadLayoutGrid         // Direct assignment
				g.finalBestSolution.Grid = g.grid // Reset solutions
				g.finalBestSolution.Path = nil
				g.finalBestSolution.Profit = -1.0
				g.intermediateBestSolution = g.finalBestSolution
				g.maxLenUsedForFinalSolution = 0
				g.validRiverStarts = nil
				g.selectedRiverStart = game.Coordinate{}
				fmt.Println("Returning to road editing from results.")
				g.updateCalculationStatus()
				g.updateButtonsForState() // Ensure buttons refresh for the new state
			},
		})

	case StateCalculating:
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Stop Calculation",
			OnClick: func(g *Game) {
				fmt.Printf("[SIMPLIFIED DEBUG] Stop Calculation button clicked. Current state: %v, g.stopCalcChannel: %p\\n", g.gameState, g.stopCalcChannel)
				if g.gameState == StateCalculating {
					if g.stopCalcChannel != nil {
						fmt.Println("[SIMPLIFIED DEBUG] Closing stopCalcChannel.")
						close(g.stopCalcChannel)
						// The goroutine is expected to see this, change state to ShowingResult,
						// and then that goroutine will call updateButtonsForState() and updateCalculationStatus().
						g.calculationStatus = "Stopping..."
					} else {
						fmt.Println("[SIMPLIFIED DEBUG] stopCalcChannel is nil, but was in StateCalculating. Forcing to ShowingResult.")
						// This case may occur if the goroutine finished and nulled the channel just before stop was clicked.
						g.gameState = StateShowingResult
						g.finalBestSolution.Grid = g.roadLayoutGrid // Default to road layout
						g.finalBestSolution.Path = nil
						g.finalBestSolution.Profit = -1.0
						g.updateButtonsForState()
						g.updateCalculationStatus()
					}
				} // Closes if g.gameState == StateCalculating
			},
		})

	case StateShowingResult:
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Recalculate (New Max Len)",
			OnClick: func(g *Game) {
				// Ensure selectedRiverStart is valid (it should be from the previous solution)
				if (g.selectedRiverStart.X == 0 && g.selectedRiverStart.Y == 0) && len(g.finalBestSolution.Path) > 0 {
					g.selectedRiverStart = g.finalBestSolution.Path[0] // Recover if somehow lost
				}

				isValidSourceForRecalc := false
				tempValidStarts := g.roadLayoutGrid.GetValidRiverStarts() // Recalculate based on original road layout
				for _, vs := range tempValidStarts {
					if vs.X == g.selectedRiverStart.X && vs.Y == g.selectedRiverStart.Y {
						isValidSourceForRecalc = true
						break
					}
				}

				if isValidSourceForRecalc {
					fmt.Printf("Recalculating with MaxLen: %d for start (%d, %d)\\n", g.currentMaxRiverLength, g.selectedRiverStart.X, g.selectedRiverStart.Y)
					g.gameState = StateCalculating
					g.updateButtonsForState() // Ensure Stop button appears immediately
					g.calculationStartTime = time.Now()

					// Initialize for iterative re-calculation
					g.isIterativeCalculationActive = true
					g.overallBestSolutionInIterativeRun = game.RiverPathSolution{Grid: g.roadLayoutGrid, Profit: -1.0, Path: nil}
					g.intermediateBestSolution = g.overallBestSolutionInIterativeRun

					g.stopCalcChannel = make(chan struct{})

					gridForCalc := g.roadLayoutGrid
					startNode := g.selectedRiverStart
					stopChan := g.stopCalcChannel
					userSelectedMaxLenForRecalc := g.currentMaxRiverLength
					disableCrossAdjacencyForRecalc := g.DisableCrossRiverAdjacency

					g.lengthUsedForCurrentCalculation = userSelectedMaxLenForRecalc // Store the user's target max length

					fmt.Printf("[DEBUG] Launching iterative re-calculation goroutine. Target MaxLen: %d, stopChan: %p, DisableCrossAdj: %t\n", userSelectedMaxLenForRecalc, stopChan, disableCrossAdjacencyForRecalc)
					go func() {
						fmt.Printf("[DEBUG] Iterative Re-calc Goroutine started. Target MaxLen: %d, stopChan: %p\n", userSelectedMaxLenForRecalc, stopChan)

						progressCb := func(intermediateSolutionForCurrentLength game.RiverPathSolution) {
							g.mu.Lock()
							// Update intermediate best for the *current length* being tested
							if intermediateSolutionForCurrentLength.Profit > g.intermediateBestSolution.Profit || g.intermediateBestSolution.Path == nil {
								g.intermediateBestSolution = intermediateSolutionForCurrentLength
							}
							// If this intermediate result is also better than the *overall best* found so far in this iterative run, update overall best
							if intermediateSolutionForCurrentLength.Profit > g.overallBestSolutionInIterativeRun.Profit {
								g.overallBestSolutionInIterativeRun = intermediateSolutionForCurrentLength
							}
							g.updateCalculationStatus()
							g.mu.Unlock()
						}
						// No local iterativeOverallBest needed

						for lengthToTest := minRiverLength; lengthToTest <= userSelectedMaxLenForRecalc; lengthToTest++ {
							select {
							case <-stopChan:
								fmt.Println("[DEBUG] Iterative re-calc loop: stopChan closed before testing length", lengthToTest)
								goto endOfRecalculation
							default:
							}

							g.mu.Lock()
							g.currentLengthBeingTested = lengthToTest
							g.intermediateBestSolution = game.RiverPathSolution{Grid: gridForCalc, Profit: -1.0, Path: nil} // Reset for current length
							g.updateCalculationStatus()
							g.mu.Unlock()

							fmt.Printf("[DEBUG] Iterative Re-calc Goroutine: Testing length %d\n", lengthToTest)
							localGridCopy := gridForCalc
							_, errThisLength := localGridCopy.FindOptimalRiverAndForests(startNode, lengthToTest, progressCb, stopChan, disableCrossAdjacencyForRecalc)

							g.mu.Lock()
							if errThisLength == nil {
								// progressCb has already updated g.intermediateBestSolution with the best for this length,
								// and g.overallBestSolutionInIterativeRun if it was a new global best.
							} else if errThisLength.Error() == "search stopped by user" {
								// progressCb would have updated g.overallBestSolutionInIterativeRun if the partial result was a new global best.
								g.mu.Unlock()
								goto endOfRecalculation
							} else {
								fmt.Printf("[DEBUG] Error during re-calc testing length %d: %v\n", lengthToTest, errThisLength)
							}
							g.updateCalculationStatus()
							g.mu.Unlock()
						}

					endOfRecalculation:
						g.mu.Lock()
						userForcedStop := false
						select {
						case <-stopChan:
							userForcedStop = true
						default:
						}

						g.isIterativeCalculationActive = false
						if g.stopCalcChannel == stopChan || (userForcedStop && g.stopCalcChannel == stopChan) {
							g.gameState = StateShowingResult
							if userForcedStop {
								fmt.Println("Iterative re-calculation stopped. Showing best overall result found.")
							} else { // Successful completion
								fmt.Printf("Iterative Goroutine (Re-calc): All lengths tested. Overall Best Profit: %.2f%%. Path Len: %d\n", g.overallBestSolutionInIterativeRun.Profit*100, len(g.overallBestSolutionInIterativeRun.Path))
							}
							// g.overallBestSolutionInIterativeRun now holds the true overall best.
							g.finalBestSolution = g.overallBestSolutionInIterativeRun
							if g.finalBestSolution.Profit < 0 {
								g.finalBestSolution.Grid = g.roadLayoutGrid
								g.finalBestSolution.Path = nil
							}
							g.maxLenUsedForFinalSolution = len(g.finalBestSolution.Path)
							if !userForcedStop { // Only update main display grid on natural completion
								g.grid = g.finalBestSolution.Grid
							}
							g.intermediateBestSolution = g.finalBestSolution

							if g.stopCalcChannel == stopChan && !userForcedStop {
								close(g.stopCalcChannel)
							}
							g.stopCalcChannel = nil
						} else {
							fmt.Println("Goroutine for an older re-calc finished or was preempted. No game state update.")
						}
						g.updateButtonsForState()
						g.updateCalculationStatus()
						g.mu.Unlock()
					}()

				} else {
					g.calculationStatus = fmt.Sprintf("Cannot re-calc: Original start (%d,%d) invalid. Esc to re-pick.", g.selectedRiverStart.X, g.selectedRiverStart.Y)
				}
				g.updateCalculationStatus()
			},
		})
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Change River Start",
			OnClick: func(g *Game) {
				g.gameState = StatePlacingRiverSource
				g.grid = g.roadLayoutGrid // Direct assignment
				g.validRiverStarts = g.roadLayoutGrid.GetValidRiverStarts()
				g.intermediateBestSolution.Grid = g.roadLayoutGrid // Direct assignment
				g.intermediateBestSolution.Path = nil
				g.intermediateBestSolution.Profit = -1.0
				g.finalBestSolution = g.intermediateBestSolution // Clear previous final solution
				g.maxLenUsedForFinalSolution = 0
				g.selectedRiverStart = game.Coordinate{} // Clear selected start
				fmt.Println("Returning to River Source Selection.")
				g.updateCalculationStatus()
				g.updateButtonsForState() // Ensure buttons refresh for the new state
			},
		})
		g.buttons = append(g.buttons, Button{
			Rect: image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
			Text: "Edit Road Layout",
			OnClick: func(g *Game) {
				g.gameState = StatePlacingRoad
				g.grid = g.roadLayoutGrid         // Direct assignment
				g.finalBestSolution.Grid = g.grid // Reset solutions
				g.finalBestSolution.Path = nil
				g.finalBestSolution.Profit = -1.0
				g.intermediateBestSolution = g.finalBestSolution
				g.maxLenUsedForFinalSolution = 0
				g.validRiverStarts = nil
				g.selectedRiverStart = game.Coordinate{}
				fmt.Println("Returning to road editing from results.")
				g.updateCalculationStatus()
				g.updateButtonsForState() // Ensure buttons refresh for the new state
			},
		})
	}

	// "Reset All (Clear Map)" button is always available
	g.buttons = append(g.buttons, Button{
		Rect:    image.Rect(buttonMinX, 0, buttonMaxX, 0), // Y will be set in Draw
		Text:    "Reset All (Clear Map)",
		OnClick: func(g *Game) { g.resetButtonAction("Full") },
	})
}

func (g *Game) resetButtonAction(resetType string) {
	// NOTE: g.mu is assumed to be HELD by the caller (e.g., the Update method)
	// Do not attempt to lock/unlock g.mu within this function.

	// Part 1: Signal the calculation goroutine to stop, if active
	if g.stopCalcChannel != nil {
		// Non-blocking check if channel is already closed to prevent panic on double close.
		select {
		case <-g.stopCalcChannel:
			// Channel was already closed.
		default:
			// Channel is not closed, so close it now.
			close(g.stopCalcChannel)
		}
		// Set the game's reference to nil. The goroutine has its own copy.
		g.stopCalcChannel = nil
		fmt.Printf("Calculation stopped due to %s Reset.\\n", resetType)
	}

	// Part 2: Reset game state fields
	fmt.Printf("Resetting game to %s state.\\n", resetType)

	switch resetType {
	case "Full":
		g.grid = game.NewGrid() // Create a fresh grid
		g.roadLayoutGrid = game.NewGrid()
		g.gameState = StatePlacingRoad
		g.currentMaxRiverLength = defaultInitialRiverLength
		g.lengthUsedForCurrentCalculation = defaultInitialRiverLength // Reset this as well
		g.maxLenUsedForFinalSolution = 0
		g.DisableCrossRiverAdjacency = false

		// Reset solution holders, ensuring their grids point to the new empty grid
		newEmptySolution := game.RiverPathSolution{Grid: g.grid, Profit: -1.0, Path: nil}
		g.finalBestSolution = newEmptySolution
		g.intermediateBestSolution = newEmptySolution
		g.overallBestSolutionInIterativeRun = newEmptySolution

		g.validRiverStarts = nil
		g.selectedRiverStart = game.Coordinate{}
		g.isIterativeCalculationActive = false // Reset iterative calculation state
		g.currentLengthBeingTested = 0

	case "ToRiverSource": // This case might be less used or need similar care if callable during calculation
		// Assuming this is typically called when not actively calculating, or the stop channel logic above handles it.
		g.gameState = StatePlacingRiverSource
		g.grid = g.roadLayoutGrid // Show the road layout
		g.validRiverStarts = g.roadLayoutGrid.GetValidRiverStarts()

		cleanSolutionForSource := game.RiverPathSolution{Grid: g.roadLayoutGrid, Profit: -1.0, Path: nil}
		g.intermediateBestSolution = cleanSolutionForSource
		g.finalBestSolution = cleanSolutionForSource
		g.overallBestSolutionInIterativeRun = cleanSolutionForSource

		g.maxLenUsedForFinalSolution = 0
		g.isIterativeCalculationActive = false // Reset iterative calculation state
		g.currentLengthBeingTested = 0
		// selectedRiverStart is intentionally NOT cleared here, as user might want to reuse previous start if coming from results
		// However, for a general "ToRiverSource" reset, clearing it might be more consistent.
		// For now, matching existing behavior where it might persist from a previous calculation context.
	}
	g.updateButtonsForState()   // Refresh buttons for the new state
	g.updateCalculationStatus() // Refresh status message
}

// detectAndCropGrid attempts to find the 12x21 game grid within a larger image and returns the cropped grid.
func detectAndCropGrid(fullImage image.Image) (image.Image, error) {
	imgBounds := fullImage.Bounds()
	imgWidth := float64(imgBounds.Dx())
	imgHeight := float64(imgBounds.Dy())

	if imgWidth == 0 || imgHeight == 0 {
		return nil, fmt.Errorf("input image has zero width or height")
	}

	// Calculate pixel coordinates from percentages
	gridStartX := int(imgWidth * gridStartXPercent)
	gridStartY := int(imgHeight * gridStartYPercent)
	gridEndX := int(imgWidth * gridEndXPercent)
	gridBottomY := int(imgHeight * gridBottomYPercent)

	gridPixelWidth := gridEndX - gridStartX
	gridPixelHeight := gridBottomY - gridStartY

	if gridPixelWidth <= 0 || gridPixelHeight <= 0 {
		return nil, fmt.Errorf("calculated grid pixel width (%.0f) or height (%.0f) is zero or negative. Image: %.0fx%.0f. Percentages: X(%.2f-%.2f) Y(%.2f-%.2f)",
			float64(gridPixelWidth), float64(gridPixelHeight), imgWidth, imgHeight, gridStartXPercent*100, gridEndXPercent*100, gridStartYPercent*100, gridBottomYPercent*100)
	}

	// Define the crop rectangle
	cropRect := image.Rect(gridStartX, gridStartY, gridEndX, gridBottomY)

	log.Printf("Calculated crop rectangle: %+v (from ImgSize: %.0fx%.0f)", cropRect, imgWidth, imgHeight)

	// Check if cropRect is valid and within the bounds of the original image
	if !cropRect.In(imgBounds) && (cropRect.Min.X != imgBounds.Min.X || cropRect.Min.Y != imgBounds.Min.Y || cropRect.Max.X != imgBounds.Max.X || cropRect.Max.Y != imgBounds.Max.Y) {
		// It's okay if the calculated rect is IDENTICAL to imgBounds (e.g. if percentages are 0-100 for a pre-cropped image)
		// But if it's different AND not fully contained, it's an error.
		// We also need to handle potential off-by-one from float to int conversions if rect is almost full image size
		// A more lenient check might be needed if rounding causes issues near image edges
		actualImgBoundsForCheck := imgBounds
		if cropRect.Min.X >= actualImgBoundsForCheck.Max.X ||
			cropRect.Min.Y >= actualImgBoundsForCheck.Max.Y ||
			cropRect.Max.X <= actualImgBoundsForCheck.Min.X ||
			cropRect.Max.Y <= actualImgBoundsForCheck.Min.Y ||
			cropRect.Min.X < actualImgBoundsForCheck.Min.X ||
			cropRect.Min.Y < actualImgBoundsForCheck.Min.Y ||
			cropRect.Max.X > actualImgBoundsForCheck.Max.X ||
			cropRect.Max.Y > actualImgBoundsForCheck.Max.Y {
			return nil, fmt.Errorf("calculated crop rectangle %+v is outside image bounds %+v", cropRect, actualImgBoundsForCheck)
		}
		log.Printf("Warning: Crop rectangle %+v not strictly 'In' image bounds %+v, but seems valid due to edge calculations.", cropRect, actualImgBoundsForCheck)
	}

	// Crop the image
	subImager, ok := fullImage.(interface {
		SubImage(r image.Rectangle) image.Image
	})
	if !ok {
		return nil, fmt.Errorf("image type does not support SubImage operation")
	}
	croppedImage := subImager.SubImage(cropRect)
	log.Printf("Successfully cropped grid using percentages. Original: %.0fx%.0f, CropRect: %+v, Cropped: %dx%d",
		imgWidth, imgHeight, cropRect, croppedImage.Bounds().Dx(), croppedImage.Bounds().Dy())
	return croppedImage, nil
}

func (g *Game) handleDetectRoadFromImage() {
	filePath, err := dialog.File().Filter("PNG Images", "png").Load()
	if err != nil {
		if err == dialog.Cancelled {
			log.Println("File selection cancelled.")
		} else {
			log.Printf("Error opening file dialog: %v", err)
			g.calculationStatus = "Error: Could not open image."
		}
		return
	}

	log.Printf("Selected file: %s", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error opening image file '%s': %v", filePath, err)
		g.calculationStatus = fmt.Sprintf("Error: Failed to open %s", filePath)
		return
	}
	defer file.Close()

	fullImg, _, err := image.Decode(file) // Underscore for format string as we don't need it
	if err != nil {
		log.Printf("Error decoding image file '%s': %v", filePath, err)
		g.calculationStatus = fmt.Sprintf("Error: Failed to decode %s", filePath)
		return
	}

	// New Step: Detect and crop the grid from the full image
	img, err := detectAndCropGrid(fullImg)
	if err != nil {
		log.Printf("Error detecting/cropping game grid: %v", err)
		g.calculationStatus = fmt.Sprintf("Grid Detect Err: %v", err)
		return // Don't proceed if grid detection failed
	}

	bounds := img.Bounds()
	imgWidth := float64(bounds.Dx())  // This is cropped image width
	imgHeight := float64(bounds.Dy()) // This is cropped image height

	// Calculate cell dimensions from the image size.
	cellWidth := imgWidth / float64(game.GridWidth)
	cellHeight := imgHeight / float64(game.GridHeight)

	if cellWidth <= 0 || cellHeight <= 0 {
		log.Printf("Error: Image dimensions (%dx%d) result in zero or negative cell size.", bounds.Dx(), bounds.Dy())
		g.calculationStatus = "Error: Invalid image dimensions for grid."
		return
	}

	detectedRoadTiles := []game.Coordinate{}

	// --- New brightness-based road detection ---
	// 1. Determine the reference brightness from the top-left tile (0,0)
	const sampleAreaSize = 4 // Sample a 4x4 area

	// Calculate center of the top-left tile (0,0) in terms of image content coordinates
	// Shifted sampling point closer to top-left (0.3, 0.3 relative offset)
	targetCellX_ref := int((0.0 + 0.3) * cellWidth)
	targetCellY_ref := int((0.0 + 0.3) * cellHeight)

	// Define the 4x4 sampling rectangle for the reference tile, relative to image content top-left (0,0)
	referenceRect := image.Rect(
		targetCellX_ref-sampleAreaSize/2,
		targetCellY_ref-sampleAreaSize/2,
		targetCellX_ref+sampleAreaSize/2,
		targetCellY_ref+sampleAreaSize/2,
	)
	referenceBrightness := getAverageBrightness(img, referenceRect)
	// log.Printf("Reference brightness (tile 0,0): %.2f from rect %+v", referenceBrightness, referenceRect) // Removed unnecessary log

	// 2. Iterate through all tiles and compare their brightness to the reference
	//    excluding the bottom row (y from 0 to game.GridHeight-2)
	for y := 0; y < game.GridHeight-1; y++ {
		for x := 0; x < game.GridWidth; x++ {
			// Calculate the center pixel of the current cell in the image content
			// Shifted sampling point closer to top-left (0.3, 0.3 relative offset)
			sampleCX := int((float64(x) + 0.3) * cellWidth)
			sampleCY := int((float64(y) + 0.3) * cellHeight)

			// Define the 4x4 sampling rectangle for the current tile, relative to image content top-left (0,0)
			currentTileSampleRect := image.Rect(
				sampleCX-sampleAreaSize/2,
				sampleCY-sampleAreaSize/2,
				sampleCX+sampleAreaSize/2,
				sampleCY+sampleAreaSize/2,
			)

			currentTileBrightness := getAverageBrightness(img, currentTileSampleRect)

			// If current tile is brighter than reference + threshold, consider it a road
			if currentTileBrightness > referenceBrightness+brightnessDifferenceThreshold {
				detectedRoadTiles = append(detectedRoadTiles, game.Coordinate{X: x, Y: y})
				// log.Printf("Tile (%d,%d) is ROAD. Brightness: %.2f (Ref: %.2f + Thresh: %.2f). Rect: %+v", x, y, currentTileBrightness, referenceBrightness, brightnessDifferenceThreshold, currentTileSampleRect) // Removed unnecessary log
			} else {
				// log.Printf("Tile (%d,%d) is NOT ROAD. Brightness: %.2f (Ref: %.2f + Thresh: %.2f). Rect: %+v", x, y, currentTileBrightness, referenceBrightness, brightnessDifferenceThreshold, currentTileSampleRect) // Removed unnecessary log
			}
		}
	}
	// --- End new brightness-based road detection ---

	g.grid = game.NewGrid() // Clear existing grid before applying new roads
	g.grid.SetRoad(detectedRoadTiles)

	// Update related game state after road detection
	g.roadLayoutGrid = g.grid // Store this as the base road layout for calculations
	g.finalBestSolution.Grid = g.grid
	g.finalBestSolution.Profit = -1.0
	g.finalBestSolution.Path = nil
	g.intermediateBestSolution = g.finalBestSolution
	g.overallBestSolutionInIterativeRun = game.RiverPathSolution{Grid: g.grid, Profit: -1.0, Path: nil}

	log.Printf("Detected %d road tiles from image.", len(detectedRoadTiles))
	g.calculationStatus = fmt.Sprintf("Detected %d road tiles. Finalize or edit.", len(detectedRoadTiles))
	g.updateButtonsForState()   // To refresh UI if needed (e.g., button states)
	g.updateCalculationStatus() // To show new status
}

// processDetectedImage takes a captured/loaded image, crops it, and runs road detection.
// It updates the game state with the detected road layout.
func (g *Game) processDetectedImage(fullImg image.Image, sourceDescription string) {
	img, err := detectAndCropGrid(fullImg)
	if err != nil {
		log.Printf("Error detecting/cropping game grid from %s: %v", sourceDescription, err)
		g.calculationStatus = fmt.Sprintf("Grid Detect Err from %s: %v", sourceDescription, err)
		return
	}

	bounds := img.Bounds()
	imgWidth := float64(bounds.Dx())
	imgHeight := float64(bounds.Dy())

	cellWidth := imgWidth / float64(game.GridWidth)
	cellHeight := imgHeight / float64(game.GridHeight)

	if cellWidth <= 0 || cellHeight <= 0 {
		log.Printf("Error: Image dimensions (%dx%d) from %s result in zero or negative cell size.", bounds.Dx(), bounds.Dy(), sourceDescription)
		g.calculationStatus = fmt.Sprintf("Error: Invalid image dimensions for grid from %s.", sourceDescription)
		return
	}

	detectedRoadTiles := []game.Coordinate{}

	const sampleAreaSize = 4
	targetCellX_ref := int((0.0 + 0.3) * cellWidth)
	targetCellY_ref := int((0.0 + 0.3) * cellHeight)
	referenceRect := image.Rect(
		targetCellX_ref-sampleAreaSize/2,
		targetCellY_ref-sampleAreaSize/2,
		targetCellX_ref+sampleAreaSize/2,
		targetCellY_ref+sampleAreaSize/2,
	)
	referenceBrightness := getAverageBrightness(img, referenceRect)

	for y := 0; y < game.GridHeight-1; y++ {
		for x := 0; x < game.GridWidth; x++ {
			sampleCX := int((float64(x) + 0.3) * cellWidth)
			sampleCY := int((float64(y) + 0.3) * cellHeight)
			currentTileSampleRect := image.Rect(
				sampleCX-sampleAreaSize/2,
				sampleCY-sampleAreaSize/2,
				sampleCX+sampleAreaSize/2,
				sampleCY+sampleAreaSize/2,
			)
			currentTileBrightness := getAverageBrightness(img, currentTileSampleRect)
			if currentTileBrightness > referenceBrightness+brightnessDifferenceThreshold {
				detectedRoadTiles = append(detectedRoadTiles, game.Coordinate{X: x, Y: y})
			}
		}
	}

	g.grid = game.NewGrid()
	g.grid.SetRoad(detectedRoadTiles)
	g.roadLayoutGrid = g.grid
	g.finalBestSolution.Grid = g.grid
	g.finalBestSolution.Profit = -1.0
	g.finalBestSolution.Path = nil
	g.intermediateBestSolution = g.finalBestSolution
	g.overallBestSolutionInIterativeRun = game.RiverPathSolution{Grid: g.grid, Profit: -1.0, Path: nil}

	log.Printf("Detected %d road tiles from %s.", len(detectedRoadTiles), sourceDescription)
	g.calculationStatus = fmt.Sprintf("Detected %d road tiles from %s. Finalize or edit.", len(detectedRoadTiles), sourceDescription)
	g.updateButtonsForState()
	g.updateCalculationStatus()
}

// abs is a helper function for absolute integer value.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// getAverageBrightness calculates the average brightness of pixels within a given rectangle in an image.
// The relativeRect's coordinates are 0-indexed relative to the logical top-left of the img's content.
func getAverageBrightness(img image.Image, relativeRect image.Rectangle) float64 {
	var totalBrightness float64
	var count int

	imgBounds := img.Bounds()
	imgContentWidth := imgBounds.Dx()
	imgContentHeight := imgBounds.Dy()

	// Iterate over the pixels in the relativeRect
	for ry := relativeRect.Min.Y; ry < relativeRect.Max.Y; ry++ {
		for rx := relativeRect.Min.X; rx < relativeRect.Max.X; rx++ {
			// Check if the relative coordinates (rx, ry) are within the image's content dimensions
			if rx >= 0 && rx < imgContentWidth && ry >= 0 && ry < imgContentHeight {
				// Convert relative (rx, ry) to absolute coordinates for img.At()
				absX := imgBounds.Min.X + rx
				absY := imgBounds.Min.Y + ry

				pixelColor := img.At(absX, absY)
				r, g, b, _ := pixelColor.RGBA() // Returns values in [0, 0xffff] range

				// Convert to 0-255 range
				r8 := uint8(r >> 8)
				g8 := uint8(g >> 8)
				b8 := uint8(b >> 8)

				// Calculate brightness for this pixel (simple average)
				brightness := (float64(r8) + float64(g8) + float64(b8)) / 3.0
				totalBrightness += brightness
				count++
			}
		}
	}

	if count == 0 {
		return 0.0 // Avoid division by zero; or handle as an error
	}
	return totalBrightness / float64(count)
}

// handleDetectRoadFromClipboard attempts to read an image from the clipboard
// and then process it for road detection.
func (g *Game) handleDetectRoadFromClipboard() {
	log.Println("Attempting to detect road from clipboard using golang.design/x/clipboard...")

	err := clipboard.Init()
	if err != nil {
		log.Printf("Error initializing clipboard (golang.design/x/clipboard): %v", err)
		g.calculationStatus = fmt.Sprintf("Clipboard Init Err: %v", err)
		g.updateCalculationStatus()
		return
	}

	// Read image data from clipboard. This expects PNG encoded bytes.
	// clipboard.Read does not return an error itself; errors should be caught by Init().
	imgBytes := clipboard.Read(clipboard.FmtImage)

	if len(imgBytes) == 0 {
		log.Println("Clipboard is empty or contains no image data in a recognized format (PNG), or Init failed previously.")
		g.calculationStatus = "Error: Clipboard empty or no PNG image."
		g.updateCalculationStatus()
		return
	}

	imgReader := bytes.NewReader(imgBytes)
	clipboardImage, format, err := image.Decode(imgReader)
	if err != nil {
		log.Printf("Error decoding image from clipboard: %v. Length of data: %d", err, len(imgBytes))
		g.calculationStatus = "Error: Failed to decode clipboard image (was it PNG?)."
		g.updateCalculationStatus()
		return
	}

	log.Printf("Successfully read and decoded image from clipboard. Format: %s", format)
	g.processDetectedImage(clipboardImage, "clipboard (golang.design)")
}

// min helper function (if not already present elsewhere)
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("River Plan Optimizer")

	gameInstance := NewGame()

	if err := ebiten.RunGame(gameInstance); err != nil {
		log.Fatal(err)
	}
}
