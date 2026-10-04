package pmap

import (
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/pdecal"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/dmapi/dmicon"

	"github.com/SpaiR/imgui-go"
)

// Shows what the decal tool is going to place on the hovered tile.
// The direction comes from the cursor, so without it you would need to click to see what you get.
func (p *PaneMap) showDecalPreview() {
	decal, ok := tools.Selected().(*tools.ToolDecal)
	if !ok || !p.active || p.canvasState.HoverOutOfBounds() {
		return
	}
	prefab, dir, ok := decal.Preview()
	if !ok {
		return
	}

	vars := prefab.Vars()
	icon, _ := vars.Text("icon")
	iconState, _ := vars.Text("icon_state")
	pixelX, _ := vars.Int("pixel_x")
	pixelY, _ := vars.Int("pixel_y")
	sprite := dmicon.Cache.GetSpriteOrPlaceholderV(icon, iconState, dir)

	// From the map pixels (bottom-up) to the screen ones (top-down). Reverse of updateCanvasMousePosition.
	camera := p.canvas.Render().Camera
	toScreen := func(x, y float32) imgui.Vec2 {
		return imgui.Vec2{
			X: p.canvasControl.PosMin().X + (x+camera.ShiftX)*camera.Scale,
			Y: p.canvasControl.PosMax().Y - (y+camera.ShiftY)*camera.Scale,
		}
	}

	tile := p.canvasState.HoveredTileBounds()
	x1 := tile.X1 + float32(pixelX)
	y1 := tile.Y1 + float32(pixelY)
	x2 := x1 + float32(sprite.IconWidth())
	y2 := y1 + float32(sprite.IconHeight())

	imgui.WindowDrawList().AddImageV(
		imgui.TextureID(sprite.Texture()),
		toScreen(x1, y2), toScreen(x2, y1),
		imgui.Vec2{X: sprite.U1, Y: sprite.V1}, imgui.Vec2{X: sprite.U2, Y: sprite.V2},
		imgui.PackedColorFromVec4(pdecal.Tint(vars)),
	)
}
