package tools

import (
	"image"
	"math"
)

// Ink closer to the center than this, in pixels, doesn't point anywhere.
const aimMinOffset = 1.0

// dirInk says where a sprite has its visible pixels for one direction, relative to the sprite's center.
// The y-axis goes up, like on the canvas.
type dirInk struct {
	dir  int
	x, y float64
}

// inkOffset returns the alpha-weighted center of the cell's pixels, relative to the center of the cell.
func inkOffset(img image.Image, cell image.Rectangle) (x, y float64) {
	var sumX, sumY, weight float64
	for py := cell.Min.Y; py < cell.Max.Y; py++ {
		for px := cell.Min.X; px < cell.Max.X; px++ {
			_, _, _, alpha := img.At(px, py).RGBA()
			sumX += float64(alpha) * (float64(px) + .5)
			sumY += float64(alpha) * (float64(py) + .5)
			weight += float64(alpha)
		}
	}
	if weight == 0 {
		return 0, 0
	}

	centerX := float64(cell.Min.X+cell.Max.X) / 2
	centerY := float64(cell.Min.Y+cell.Max.Y) / 2
	return sumX/weight - centerX, centerY - sumY/weight // images go top-down, so flip the y
}

// aimDir picks the direction whose ink lies the same way from the center as the cursor does.
// The cursor is relative to the center of the tile, with the y-axis going up.
// Returns false when the sprite can't be aimed: it has a single direction, or one of them has its ink in the center.
// Skipping only the centered ones would leave them with no way to be picked.
func aimDir(inks []dirInk, cursorX, cursorY float64) (int, bool) {
	if len(inks) < 2 {
		return 0, false
	}

	bestDir, bestScore := 0, math.Inf(-1)
	for _, ink := range inks {
		length := math.Hypot(ink.x, ink.y)
		if length < aimMinOffset {
			return 0, false
		}
		// The cosine of the angle between the two, without the cursor's length, which is the same for everyone.
		if score := (ink.x*cursorX + ink.y*cursorY) / length; score > bestScore {
			bestDir, bestScore = ink.dir, score
		}
	}
	return bestDir, true
}
