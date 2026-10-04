package tools

import (
	"image"
	"image/color"
	"testing"

	"sdmm/internal/dmapi/dm"

	"github.com/stretchr/testify/assert"
)

// fill paints an opaque rectangle onto the image.
func fill(img *image.NRGBA, rect image.Rectangle) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
}

func TestInkOffset(t *testing.T) {
	// A sheet of two 32x32 cells. The first one has a line along its top edge,
	// the second one has its bottom-left quarter filled, and that's the one with a non-zero origin.
	sheet := image.NewNRGBA(image.Rect(0, 0, 64, 32))
	fill(sheet, image.Rect(0, 0, 32, 4))
	fill(sheet, image.Rect(32, 16, 48, 32))

	x, y := inkOffset(sheet, image.Rect(0, 0, 32, 32))
	assert.InDelta(t, 0, x, 1e-9)
	assert.InDelta(t, 14, y, 1e-9, "the top of the image is up")

	x, y = inkOffset(sheet, image.Rect(32, 0, 64, 32))
	assert.InDelta(t, -8, x, 1e-9)
	assert.InDelta(t, -8, y, 1e-9)

	x, y = inkOffset(image.NewNRGBA(image.Rect(0, 0, 32, 32)), image.Rect(0, 0, 32, 32))
	assert.Zero(t, x, "an empty sprite has no ink to point anywhere")
	assert.Zero(t, y)
}

func TestAimDir(t *testing.T) {
	edges := []dirInk{
		{dm.DirNorth, 0, 14},
		{dm.DirEast, 14, 0},
		{dm.DirSouth, 0, -14},
		{dm.DirWest, -14, 0},
	}
	withCorners := append([]dirInk{
		{dm.DirNortheast, 7, 7},
		{dm.DirSoutheast, 7, -7},
		{dm.DirSouthwest, -7, -7},
		{dm.DirNorthwest, -7, 7},
	}, edges...)

	tests := []struct {
		name             string
		inks             []dirInk
		cursorX, cursorY float64
		dir              int
		ok               bool
	}{
		{"top edge", edges, 2, 12, dm.DirNorth, true},
		{"left edge", edges, -12, -3, dm.DirWest, true},
		{"barely off the center still aims", edges, .5, -1.5, dm.DirSouth, true},
		{"corner, edges only", edges, 15, 13, dm.DirEast, true},
		{"corner, with corner sprites", withCorners, 15, 13, dm.DirNortheast, true},
		{"edge, with corner sprites", withCorners, -3, -14, dm.DirSouth, true},
		{"ink in the center can't be aimed", []dirInk{{dm.DirNorth, 0, .5}, {dm.DirSouth, 0, -.5}}, 0, 10, 0, false},
		{"one centered direction spoils the rest", append([]dirInk{{dm.DirNortheast, 0, 0}}, edges...), 2, 12, 0, false},
		{"one direction is not a choice", edges[:1], 2, 12, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir, ok := aimDir(tt.inks, tt.cursorX, tt.cursorY)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.dir, dir)
			}
		})
	}
}
