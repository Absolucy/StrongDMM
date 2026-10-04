package tools

import (
	"image"
	"slices"
	"strconv"
	"strings"

	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/overlay"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmmap/dmminstance"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/imguiext"
	"sdmm/internal/util"
)

const (
	// DecalRoot is the type of decals. The alternative behaviour removes it and everything under it.
	DecalRoot = "/obj/effect/turf_decal"

	// Walls and such. A decal there is never seen.
	closedTurf = "/turf/closed"
)

var (
	cardinalDirs  = []int{dm.DirNorth, dm.DirEast, dm.DirSouth, dm.DirWest}
	clockwiseDirs = []int{
		dm.DirNorth, dm.DirNortheast, dm.DirEast, dm.DirSoutheast,
		dm.DirSouth, dm.DirSouthwest, dm.DirWest, dm.DirNorthwest,
	}
)

// ToolDecal can be used to add the selected prefab turned to the part of the tile the cursor is on.
// It looks where the sprite has its pixels for every direction, so a line goes to the edge you point at.
// The direction is picked when the mouse button goes down and stays for the whole OnStart -> OnStop cycle,
// so dragging along a wall gives the same direction to every tile.
// Sprites which are centered in some of their directions can't be aimed, and are added as they are selected.
//
// Default: the prefab is placed on top, unless the tile is closed or has exactly the same one.
// With Shift: the corner piece of the prefab is placed instead, if there is a type for it.
// With Ctrl: the outline of the dragged box gets the prefab, every side and corner facing outwards.
// Alternative: all decals on the tile are removed.
type ToolDecal struct {
	tool

	editedTiles map[util.Point]bool

	// What a click would place, before it's turned: the selected prefab or its corner piece.
	source *dmmprefab.Prefab
	// Where the cursor points at. Zero when the source can't be aimed.
	aimedDir int

	// What the current OnStart -> OnStop cycle does.
	stroke  *dmmprefab.Prefab // the source, turned to the aimed direction
	erasing bool
	boxing  bool
	boxFrom util.Point
	boxTo   util.Point

	// Inks of the state we aimed the last time, to not count its pixels every frame.
	inksState *dmicon.State
	inks      []dirInk
}

func (ToolDecal) Name() string {
	return TNDecal
}

func newDecal() *ToolDecal {
	return &ToolDecal{
		editedTiles: make(map[util.Point]bool),
	}
}

// The box shows itself, so the hovered tile shouldn't be highlighted on top of it.
func (t *ToolDecal) Stale() bool {
	return !t.boxing
}

// Preview returns what a click would add right now: the prefab and the direction it would face.
func (t *ToolDecal) Preview() (prefab *dmmprefab.Prefab, dir int, ok bool) {
	if active || t.AltBehaviour() || t.source == nil {
		return nil, 0, false
	}
	if t.aimedDir != 0 {
		return t.source, t.aimedDir, true
	}
	return t.source, t.source.Vars().IntV("dir", dm.DirDefault), true
}

func (t *ToolDecal) process() {
	if !active {
		t.aim()
	}

	if t.boxing {
		ed.OverlayPushArea(t.box(), overlay.ColorToolFillTileFill, overlay.ColorToolFillTileBorder)
	}
	for coord := range t.editedTiles {
		if t.erasing {
			ed.OverlayPushTile(coord, overlay.ColorToolDeleteAltTileFill, overlay.ColorToolDeleteAltTileBorder)
		} else {
			ed.OverlayPushTile(coord, overlay.ColorToolAddTileFill, overlay.ColorToolAddTileBorder)
		}
	}
}

func (t *ToolDecal) onStart(coord util.Point) {
	if t.AltBehaviour() {
		t.erasing = true
	} else if t.source != nil {
		t.stroke = t.source
		if t.aimedDir != 0 {
			t.stroke = PrefabWithDir(t.source, t.aimedDir)
		}
		if imguiext.IsCtrlDown() {
			t.boxing = true
			t.boxFrom = coord
		}
	}
	t.onMove(coord)
}

func (t *ToolDecal) onMove(coord util.Point) {
	if t.boxing {
		t.boxTo = coord
		return
	}
	if t.editedTiles[coord] || !t.erasing && t.stroke == nil {
		return
	}
	t.editedTiles[coord] = true

	tile := ed.Dmm().GetTile(coord)
	if t.erasing {
		tile.InstancesRemoveByPath(DecalRoot)
	} else if canPlace(tile, t.stroke) {
		tile.InstancesAdd(t.stroke)
	}

	ed.UpdateCanvasByCoords([]util.Point{coord})
}

func (t *ToolDecal) onStop(util.Point) {
	if t.boxing {
		t.placeBox()
		go ed.CommitChanges("Paint Decals")
	} else if len(t.editedTiles) != 0 {
		t.editedTiles = make(map[util.Point]bool, len(t.editedTiles))
		if t.erasing {
			go ed.CommitChanges("Erase Decals")
		} else {
			go ed.CommitChanges("Paint Decals")
		}
	}
	t.stroke, t.erasing, t.boxing = nil, false, false
}

// aim updates what a click would place and where it would face.
func (t *ToolDecal) aim() {
	t.source, t.aimedDir = nil, 0

	prefab, ok := ed.SelectedPrefab()
	if !ok {
		return
	}
	t.source = prefab
	if imguiext.IsShiftDown() {
		if corner, ok := cornerOf(prefab); ok {
			t.source = corner
		}
	}

	// Mouse position, but local to the hovered tile and counted from its center.
	half := float64(dmmap.WorldIconSize) / 2
	cursorX := float64(cs.RelMouseX()%dmmap.WorldIconSize) + .5 - half
	cursorY := float64(cs.RelMouseY()%dmmap.WorldIconSize) + .5 - half

	if dir, ok := aimDir(t.inksOf(t.source), cursorX, cursorY); ok {
		t.aimedDir = dir
	}
}

func (t *ToolDecal) inksOf(prefab *dmmprefab.Prefab) []dirInk {
	state, err := prefabState(prefab)
	if err != nil {
		return nil
	}
	if state != t.inksState {
		t.inksState, t.inks = state, stateInks(state)
	}
	return t.inks
}

func (t *ToolDecal) box() util.Bounds {
	return util.Bounds{
		X1: float32(min(t.boxFrom.X, t.boxTo.X)),
		Y1: float32(min(t.boxFrom.Y, t.boxTo.Y)),
		X2: float32(max(t.boxFrom.X, t.boxTo.X)),
		Y2: float32(max(t.boxFrom.Y, t.boxTo.Y)),
	}
}

func (t *ToolDecal) placeBox() {
	box := t.box()
	x1, y1, x2, y2 := int(box.X1), int(box.Y1), int(box.X2), int(box.Y2)

	inks := t.inksOf(t.source)
	hasDiagonals := len(inks) == len(clockwiseDirs)

	var coords []util.Point
	for x := x1; x <= x2; x++ {
		for y := y1; y <= y2; y++ {
			sideX, sideY := boxSide(x1, x2, x), boxSide(y1, y2, y)
			if sideX == 0 && sideY == 0 {
				continue
			}
			coord := util.Point{X: x, Y: y, Z: t.boxFrom.Z}
			tile := ed.Dmm().GetTile(coord)
			coords = append(coords, coord)

			// A box of one row or column has no outside to face. It's a straight line, aimed as usual.
			if x1 == x2 || y1 == y2 {
				if canPlace(tile, t.stroke) {
					tile.InstancesAdd(t.stroke)
				}
				continue
			}
			for _, aim := range boxAims(sideX, sideY, hasDiagonals) {
				prefab := t.source
				if dir, ok := aimDir(inks, aim[0], aim[1]); ok {
					prefab = PrefabWithDir(t.source, dir)
				}
				if canPlace(tile, prefab) {
					tile.InstancesAdd(prefab)
				}
			}
		}
	}
	ed.UpdateCanvasByCoords(coords)
}

// boxSide tells on which side of the box the value is: -1 for the low one, 1 for the high one, 0 for in between.
func boxSide(low, high, value int) int {
	switch value {
	case low:
		return -1
	case high:
		return 1
	}
	return 0
}

// boxAims returns where to aim the decals of a tile on the box outline, as the aimDir cursor.
// A corner wants the sprite which covers both of its sides. Without one, it gets a decal per side.
func boxAims(sideX, sideY int, hasDiagonals bool) [][2]float64 {
	if sideX != 0 && sideY != 0 && !hasDiagonals {
		return [][2]float64{{float64(sideX), 0}, {0, float64(sideY)}}
	}
	return [][2]float64{{float64(sideX), float64(sideY)}}
}

// cornerOf returns the corner piece for the prefab: ".../line" has it in ".../corner", others in their "/corner" subtype.
func cornerOf(prefab *dmmprefab.Prefab) (*dmmprefab.Prefab, bool) {
	return dmmap.PrefabStorage.InitialV(strings.TrimSuffix(prefab.Path(), "/line") + "/corner")
}

// PrefabWithDir returns the prefab turned to the provided direction.
// The direction which the type has by default is not stored, so it won't get to the map file.
func PrefabWithDir(prefab *dmmprefab.Prefab, dir int) *dmmprefab.Prefab {
	vars := prefab.Vars()
	if dir == dmmap.PrefabStorage.Initial(prefab.Path()).Vars().IntV("dir", dm.DirDefault) {
		vars = dmvars.Delete(vars, "dir")
	} else {
		vars = dmvars.Set(vars, "dir", strconv.Itoa(dir))
	}
	return dmmap.PrefabStorage.Get(prefab.Path(), vars)
}

// RotatedPrefab returns the prefab turned clockwise to the next direction its sprite has.
func RotatedPrefab(prefab *dmmprefab.Prefab) *dmmprefab.Prefab {
	state, err := prefabState(prefab)
	if err != nil {
		return prefab
	}
	dirs := spriteDirs(state)
	if len(dirs) == 0 {
		return prefab
	}
	// A direction the sprite doesn't have gives -1 here, which starts us from the first one.
	current := slices.Index(dirs, prefab.Vars().IntV("dir", dm.DirDefault))
	return PrefabWithDir(prefab, dirs[(current+1)%len(dirs)])
}

func prefabState(prefab *dmmprefab.Prefab) (*dmicon.State, error) {
	icon, _ := prefab.Vars().Text("icon")
	iconState, _ := prefab.Vars().Text("icon_state")
	return dmicon.Cache.GetState(icon, iconState)
}

func spriteDirs(state *dmicon.State) []int {
	switch state.Dirs {
	case 4:
		return cardinalDirs
	case 8:
		return clockwiseDirs
	}
	return nil
}

func stateInks(state *dmicon.State) []dirInk {
	dirs := spriteDirs(state)
	inks := make([]dirInk, 0, len(dirs))
	for _, dir := range dirs {
		sprite := state.SpriteV(dir)
		x, y := inkOffset(sprite.Image(), image.Rect(sprite.X1, sprite.Y1, sprite.X2, sprite.Y2))
		inks = append(inks, dirInk{dir, x, y})
	}
	return inks
}

// canPlace is false for closed tiles and for the ones which have exactly this prefab already.
func canPlace(tile *dmmap.Tile, prefab *dmmprefab.Prefab) bool {
	return !slices.ContainsFunc(tile.Instances(), func(instance *dmminstance.Instance) bool {
		return instance.Prefab().Id() == prefab.Id() || dm.IsPath(instance.Prefab().Path(), closedTurf)
	})
}
