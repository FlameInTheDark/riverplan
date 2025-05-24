package main

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

// UI Button constants
const (
	panelWidth    = 240 // Increased for more space
	buttonHeight  = 30
	buttonMargin  = 10
	buttonPadding = 5
	textOffsetY   = 5 // Small offset for text within buttons
)

// Button struct for UI elements
type Button struct {
	Rect    image.Rectangle
	Text    string
	OnClick func(g *Game) // Action to perform on click
}

// wrapText is a helper function to break long strings into multiple lines.
func wrapText(input string, maxWidth int, lineHeight int) []string {
	var lines []string
	var currentLine string
	currentLineWidth := 0

	// Rough estimate of character width for basicfont.Face7x13
	charWidth := text.BoundString(basicfont.Face7x13, "0").Dx() // Use text.BoundString for width of '0'
	if charWidth == 0 {
		charWidth = 7
	} // Fallback if metrics are weird

	for _, r := range input {
		// Handle newline characters explicitly
		if r == '\n' {
			lines = append(lines, currentLine)
			currentLine = ""
			currentLineWidth = 0
			continue
		}

		if currentLineWidth+charWidth > maxWidth {
			lines = append(lines, currentLine)
			currentLine = string(r)
			currentLineWidth = charWidth
		} else {
			currentLine += string(r)
			currentLineWidth += charWidth
		}
	}
	if len(currentLine) > 0 {
		lines = append(lines, currentLine)
	}
	return lines
}

// updatePanelControlRects calculates the screen positions for custom UI controls in the panel.
func (g *Game) updatePanelControlRects() {
	fontFace := basicfont.Face7x13
	fontHeight := fontFace.Metrics().Height.Ceil()
	if fontHeight == 0 {
		fontHeight = 13
	} // Fallback

	panelTopY := buttonMargin
	currentY := panelTopY + 20 + buttonMargin // After "River Planner" text

	statusLines := wrapText(g.calculationStatus, panelWidth-(2*buttonMargin), fontHeight)
	currentY += len(statusLines) * fontHeight
	currentY += buttonMargin // Space after status text

	// --- Scrollbar for Max River Len ---
	scrollBarMarginHorizontal := buttonMargin + 5
	scrollBarWidth := panelWidth - (2 * scrollBarMarginHorizontal)
	scrollBarHeight := 10
	thumbWidth := 15
	thumbHeight := 18

	// Y position for the scrollbar (centered on a line)
	// This currentY is now after the status text.
	scrollBarLineY := currentY + thumbHeight/2 + 5 // Ensure thumb is fully visible and centered on this line

	g.scrollBarRect = image.Rect(
		scrollBarMarginHorizontal,
		scrollBarLineY-scrollBarHeight/2,
		scrollBarMarginHorizontal+scrollBarWidth,
		scrollBarLineY+scrollBarHeight/2,
	)

	// Calculate thumb position based on currentMaxRiverLength
	valRange := float64(maxRiverLengthCap - minRiverLength)
	if valRange == 0 { // Avoid division by zero if min and max are the same
		valRange = 1
	}
	percentage := float64(g.currentMaxRiverLength-minRiverLength) / valRange
	trackWidthForThumb := scrollBarWidth - thumbWidth // The range of X coords the left of the thumb can be in
	thumbMinX := g.scrollBarRect.Min.X + int(percentage*float64(trackWidthForThumb))

	g.scrollThumbRect = image.Rect(
		thumbMinX,
		scrollBarLineY-thumbHeight/2,
		thumbMinX+thumbWidth,
		scrollBarLineY+thumbHeight/2,
	)

	// The "Max River Len: X" text will be drawn above or near this scrollbar.
	// The (PgUp/PgDn) can remain below it.

	// Remove or repurpose minus/plus button rect calculations for now
	g.minusRiverLengthButtonRect = image.Rect(0, 0, 0, 0) // Effectively hide them
	g.plusRiverLengthButtonRect = image.Rect(0, 0, 0, 0)

}

func (g *Game) drawPanel(screen *ebiten.Image) {
	g.updatePanelControlRects() // Ensure rects are calculated with the most current status

	// --- Draw Panel UI ---
	panelBg := color.RGBA{R: 30, G: 30, B: 40, A: 255} // Darker panel
	panelRect := image.Rect(0, 0, panelWidth, screenHeight)
	ebitenutil.DrawRect(screen, float64(panelRect.Min.X), float64(panelRect.Min.Y), float64(panelRect.Dx()), float64(panelRect.Dy()), panelBg)

	// Panel Text (using ebiten/text for better control if needed, for now DebugPrintAt)
	currentY := buttonMargin
	text.Draw(screen, "River Planner", basicfont.Face7x13, buttonMargin, currentY+10, color.White) // +10 for basicfont baseline
	currentY += 20 + buttonMargin

	// Wrapped status text
	statusLines := wrapText(g.calculationStatus, panelWidth-(2*buttonMargin), basicfont.Face7x13.Metrics().Height.Ceil())
	for _, line := range statusLines {
		text.Draw(screen, line, basicfont.Face7x13, buttonMargin, currentY+10, color.White)
		currentY += basicfont.Face7x13.Metrics().Height.Ceil()
	}
	currentY += buttonMargin

	// Draw the scrollbar (track and thumb)
	// g.scrollBarRect and g.scrollThumbRect are calculated by updatePanelControlRects
	// The Y position used in updatePanelControlRects for scrollBarLineY is based on currentY *after* status text.
	// So, we need to ensure currentY here in Draw reflects that baseline before drawing scrollbar hint.

	// The actual drawing of scrollbar uses its own pre-calculated Rects.
	// We need to advance currentY based on where the scrollbar *will be/was* drawn to position subsequent elements.
	// The scrollbar's effective total height for layout purposes is thumbHeight centered at its line.
	// The scrollBarLineY in updatePanelControlRects is currentY (after status) + thumbHeight/2 + 5.
	// So, the space taken by scrollbar visually ends around scrollBarLineY + thumbHeight/2.
	// Let's use the scrollBarRect's Max.Y for simplicity, then add margin.
	// Note: updatePanelControlRects determines the scrollbar's actual Y. We draw it here,
	// then correctly position the *next* element (PgUp/PgDn hint) below it.

	// Scrollbar Track
	trackColor := color.RGBA{R: 50, G: 50, B: 60, A: 255}
	ebitenutil.DrawRect(screen, float64(g.scrollBarRect.Min.X), float64(g.scrollBarRect.Min.Y), float64(g.scrollBarRect.Dx()), float64(g.scrollBarRect.Dy()), trackColor)

	// Scrollbar Thumb
	thumbColor := color.RGBA{R: 100, G: 100, B: 120, A: 255}
	if g.isDraggingScrollBar {
		thumbColor = color.RGBA{R: 130, G: 130, B: 150, A: 255} // Highlight when dragging
	}
	ebitenutil.DrawRect(screen, float64(g.scrollThumbRect.Min.X), float64(g.scrollThumbRect.Min.Y), float64(g.scrollThumbRect.Dx()), float64(g.scrollThumbRect.Dy()), thumbColor)

	// Update currentY to be below the scrollbar for the next element.
	// Use the bottom of the thumb (which is usually taller) as the reference, plus some margin.
	currentY = g.scrollThumbRect.Max.Y + 5

	text.Draw(screen, "(Use PgUp/PgDn)", basicfont.Face7x13, buttonMargin, currentY+10, color.White)
	currentY += 15 + buttonMargin // Advance Y past PgUp/PgDn hint, add a small margin before buttons

	// Draw Action Buttons (dynamically positioned)
	buttonBgColor := color.RGBA{R: 70, G: 70, B: 90, A: 255}
	buttonTextColor := color.White

	for i := range g.buttons {
		// Set the actual Y position for the button Rect just before drawing
		g.buttons[i].Rect.Min.Y = currentY
		g.buttons[i].Rect.Max.Y = currentY + buttonHeight // buttonHeight is a global const

		// Draw the button background
		ebitenutil.DrawRect(screen,
			float64(g.buttons[i].Rect.Min.X),
			float64(g.buttons[i].Rect.Min.Y),
			float64(g.buttons[i].Rect.Dx()),
			float64(g.buttons[i].Rect.Dy()),
			buttonBgColor,
		)

		// Draw the button text (centered)
		textBounds := text.BoundString(basicfont.Face7x13, g.buttons[i].Text)
		textX := g.buttons[i].Rect.Min.X + (g.buttons[i].Rect.Dx()-textBounds.Dx())/2
		textY := g.buttons[i].Rect.Min.Y + (g.buttons[i].Rect.Dy()+textBounds.Dy())/2 - textOffsetY // textOffsetY is a global const
		text.Draw(screen, g.buttons[i].Text, basicfont.Face7x13, textX, textY, buttonTextColor)

		currentY += buttonHeight + buttonMargin // Advance Y for the next button
	}

	// TPS/FPS counter at the bottom of the panel or screen
	fpsDisplayY := screenHeight - 15 // screenHeight is a global const from main.go
	text.Draw(screen, fmt.Sprintf("TPS: %.0f FPS: %.0f", ebiten.ActualTPS(), ebiten.ActualFPS()), basicfont.Face7x13, buttonMargin, fpsDisplayY, color.White)
}
