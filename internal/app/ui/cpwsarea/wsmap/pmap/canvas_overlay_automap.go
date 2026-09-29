package pmap

import (
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/canvas"
	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/overlay"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"
)

var AutomapRendering = true

func (p *PaneMap) processCanvasOverlayAutomap() {
	if !AutomapRendering || p.layer == nil {
		return
	}

	iconSize := float32(dmmap.WorldIconSize)
	for idx, t := range p.layer.Templates {
		if t.Z != p.activeLevel || t.Size() == 0 {
			continue
		}

		var borders []util.Bounds
		for _, coord := range t.Coords() {
			x := float32(coord.X-1) * iconSize
			y := float32(coord.Y-1) * iconSize

			if !t.Owns(util.Point{X: coord.X, Y: coord.Y + 1, Z: coord.Z}) {
				borders = append(borders, util.Bounds{X1: x, Y1: y + iconSize, X2: x + iconSize, Y2: y + iconSize})
			}
			if !t.Owns(util.Point{X: coord.X + 1, Y: coord.Y, Z: coord.Z}) {
				borders = append(borders, util.Bounds{X1: x + iconSize, Y1: y, X2: x + iconSize, Y2: y + iconSize})
			}
			if !t.Owns(util.Point{X: coord.X, Y: coord.Y - 1, Z: coord.Z}) {
				borders = append(borders, util.Bounds{X1: x, Y1: y, X2: x + iconSize, Y2: y})
			}
			if !t.Owns(util.Point{X: coord.X - 1, Y: coord.Y, Z: coord.Z}) {
				borders = append(borders, util.Bounds{X1: x, Y1: y, X2: x, Y2: y + iconSize})
			}

			if p.layer.IsFrozen(coord) {
				p.editor.OverlayPushTile(coord, overlay.ColorAutomapFrozenFill, overlay.ColorAutomapFrozenBorder)
			}
		}

		p.canvasOverlay.PushAreaBorder(canvas.OverlayAreaBorder{
			Borders_: borders,
			Color_:   overlay.ColorAutomapTemplate(idx),
		})
	}
}
