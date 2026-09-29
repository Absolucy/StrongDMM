package automap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/util"
)

const (
	noopTurf = "/turf/template_noop"
	noopArea = "/area/template_noop"
)

// Layer is every automapper template stamped onto one open station map.
// The live map shows the stamped result, the layer remembers who owns which tile so Save can split it back.
type Layer struct {
	cfg *Config

	// the station map as it is on disk, so tiles under templates can be put back on save
	base *dmmap.Dmm

	Templates []*Template
	Warnings  []string
}

type Template struct {
	Name      string
	Directory string
	Trait     string
	Z         int

	path   string
	origin util.Point

	footprint map[util.Point]*footprintTile

	// not on disk: never saved yet, or deleted by a save after its footprint went empty
	isNew            bool
	footprintChanged bool
}

type footprintTile struct {
	// what the template file has here; nil for tiles handed over by ownership edits
	source dmmdata.Prefabs
	// the live tile right after stamping (or the last save), to tell edits apart from untouched tiles
	stamped dmmdata.Prefabs
}

// Load stamps every template meant for dmm onto it. The layer is nil when the project has no automapper.
func Load(dme *dmenv.Dme, dmm *dmmap.Dmm) (*Layer, error) {
	cfg, err := FindConfig(dme.RootDir)
	if err != nil || cfg == nil {
		return nil, err
	}
	if dme.Objects[noopTurf] == nil || dme.Objects[noopArea] == nil {
		return nil, fmt.Errorf("automapper config found, but %s or %s is missing from the environment", noopTurf, noopArea)
	}

	base := dmm.Copy()
	l := &Layer{cfg: cfg, base: &base}

	for _, entry := range cfg.Entries {
		if !strings.EqualFold(entry.RequiredMap, dmm.Name) {
			continue
		}
		t, err := l.loadTemplate(dme, dmm, entry)
		if err != nil {
			l.Warnings = append(l.Warnings, fmt.Sprintf("%s: %v", entry.Name, err))
			continue
		}
		l.Templates = append(l.Templates, t)
	}

	l.stamp(dmm)
	return l, nil
}

func (l *Layer) loadTemplate(dme *dmenv.Dme, dmm *dmmap.Dmm, entry *Entry) (*Template, error) {
	if len(entry.MapFiles) != 1 {
		return nil, fmt.Errorf("has %d map_files, only templates with exactly one are supported", len(entry.MapFiles))
	}
	if len(entry.Coordinates) != 3 {
		return nil, fmt.Errorf("coordinates should be [x, y, z], got %v", entry.Coordinates)
	}

	path := l.cfg.TemplatePath(entry)
	data, err := dmmdata.New(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	source, unknown := dmmap.New(dme, data, "")
	if len(unknown) > 0 {
		l.Warnings = append(l.Warnings, fmt.Sprintf("%s: %d unknown types, they'd be dropped if this template is saved", entry.Name, len(unknown)))
	}

	origin := util.Point{X: entry.Coordinates[0], Y: entry.Coordinates[1], Z: entry.Coordinates[2]}
	top := util.Point{X: origin.X + source.MaxX - 1, Y: origin.Y + source.MaxY - 1, Z: origin.Z}
	// the game refuses a template that doesn't fit whole, so we do too
	if !dmm.HasTile(origin) || !dmm.HasTile(top) {
		return nil, fmt.Errorf("doesn't fit on the map (%v to %v), the game won't load it either", origin, top)
	}

	t := &Template{
		Name:      entry.Name,
		Directory: entry.Directory,
		Trait:     entry.TraitName,
		Z:         origin.Z,
		path:      path,
		origin:    origin,
		footprint: make(map[util.Point]*footprintTile),
	}
	for y := 1; y <= source.MaxY; y++ {
		for x := 1; x <= source.MaxX; x++ {
			prefabs := source.GetTile(util.Point{X: x, Y: y, Z: 1}).Instances().Prefabs()
			if isPureNoop(prefabs) {
				continue
			}
			t.footprint[t.toStation(x, y)] = &footprintTile{source: prefabs}
		}
	}
	return t, nil
}

// stamp mirrors the game's load order. Every tile a template gives a real turf gets skipped by the
// station loader entirely, then templates load in config order, each one only adding objects and
// swapping the turf/area where it isn't noop. Doing it per tile against the live tile can't work,
// since there's no telling station leftovers from an earlier template's objects.
func (l *Layer) stamp(dmm *dmmap.Dmm) {
	for _, t := range l.Templates {
		for coord, ft := range t.footprint {
			if !isNoop(turfsOf(ft.source), noopTurf) {
				dmm.GetTile(coord).InstancesSet(dmmdata.Prefabs{dmmap.BaseTurf, dmmap.BaseArea})
			}
		}
	}

	for _, t := range l.Templates {
		for coord, ft := range t.footprint {
			tile := dmm.GetTile(coord)
			tile.InstancesSet(applyTemplateTile(ft.source, tile.Instances().Prefabs()))
		}
	}

	for _, t := range l.Templates {
		for coord, ft := range t.footprint {
			ft.stamped = dmm.GetTile(coord).Instances().Prefabs()
		}
	}
}

// applyTemplateTile adds the template's objects and swaps turf and area only where they aren't noop.
func applyTemplateTile(template, live dmmdata.Prefabs) dmmdata.Prefabs {
	turfs, areas := turfsOf(live), areasOf(live)
	if t := turfsOf(template); !isNoop(t, noopTurf) {
		turfs = t
	}
	if a := areasOf(template); !isNoop(a, noopArea) {
		areas = a
	}

	result := append(movablesOf(live), movablesOf(template)...)
	result = append(result, turfs...)
	return append(result, areas...)
}

func (t *Template) toStation(x, y int) util.Point {
	return util.Point{X: t.origin.X + x - 1, Y: t.origin.Y + y - 1, Z: t.origin.Z}
}

// Owns reports whether the tile belongs to this template's footprint.
func (t *Template) Owns(coord util.Point) bool {
	_, ok := t.footprint[coord]
	return ok
}

func (t *Template) Size() int {
	return len(t.footprint)
}

// Coords lists the footprint, for drawing it.
func (t *Template) Coords() []util.Point {
	coords := make([]util.Point, 0, len(t.footprint))
	for coord := range t.footprint {
		coords = append(coords, coord)
	}
	return coords
}

// Bounds is the footprint's bounding box, which is what the template file covers after a save.
func (t *Template) Bounds() (lo, hi util.Point) {
	first := true
	for coord := range t.footprint {
		if first {
			lo, hi = coord, coord
			first = false
			continue
		}
		lo.X, lo.Y = min(lo.X, coord.X), min(lo.Y, coord.Y)
		hi.X, hi.Y = max(hi.X, coord.X), max(hi.Y, coord.Y)
	}
	return lo, hi
}

// PendingDelete is a saved template whose footprint went empty, so the next save deletes it.
func (t *Template) PendingDelete() bool {
	return !t.isNew && len(t.footprint) == 0
}

// Hidden templates are new ones that lost all their tiles (their creation was undone), nothing to show.
func (t *Template) Hidden() bool {
	return t.isNew && len(t.footprint) == 0
}

// Dirty reports whether a save would write this template.
func (t *Template) Dirty(l *Layer, live *dmmap.Dmm) bool {
	if t.isNew || t.footprintChanged {
		return true
	}
	for coord, ft := range t.footprint {
		if ft.source == nil {
			return true
		}
		if !l.IsFrozen(coord) && !sameTile(live.GetTile(coord).Instances(), ft.stamped) {
			return true
		}
	}
	return false
}

// IsFrozen is true for tiles two templates cover. The game adds up both templates' objects there,
// so there's no telling which file an edit belongs to.
func (l *Layer) IsFrozen(coord util.Point) bool {
	owners := 0
	for _, t := range l.Templates {
		if t.Owns(coord) {
			owners++
		}
	}
	return owners > 1
}

// Owner is the template covering the tile, or nil. For frozen tiles it's the last one loaded.
func (l *Layer) Owner(coord util.Point) *Template {
	var owner *Template
	for _, t := range l.Templates {
		if t.Owns(coord) {
			owner = t
		}
	}
	return owner
}

// NewTemplate makes a template for mapName out of coords, as they currently look.
// It's only written out on the next save.
func (l *Layer) NewTemplate(name, trait, mapName string, z int, coords []util.Point) (*Template, []util.Point, error) {
	if !ValidName.MatchString(name) {
		return nil, nil, errors.New("names can only use a-z, 0-9 and _")
	}
	if l.NameTaken(name) {
		return nil, nil, fmt.Errorf("there's already a template called %s", name)
	}

	directory := l.cfg.NewTemplateDirectory(strings.TrimSuffix(mapName, filepath.Ext(mapName)))
	path := filepath.Join(l.cfg.RootDir, filepath.FromSlash(directory), name+".dmm")
	if _, err := os.Stat(path); err == nil {
		return nil, nil, fmt.Errorf("%s already exists", path)
	}

	t := &Template{
		Name:      name,
		Directory: directory,
		Trait:     trait,
		Z:         z,
		path:      path,
		footprint: make(map[util.Point]*footprintTile),
		isNew:     true,
	}
	added, err := l.AddTiles(t, coords)
	if err != nil {
		return nil, nil, err
	}
	l.Templates = append(l.Templates, t)
	return t, added, nil
}

// NameTaken counts templates that only exist in this session too, so a save can't end up with two.
func (l *Layer) NameTaken(name string) bool {
	if l.cfg.Entry(name) != nil {
		return true
	}
	for _, t := range l.Templates {
		if t.Name == name {
			return true
		}
	}
	return false
}

func (t *Template) Path() string {
	return t.path
}

// OnDisk is false for templates that were never saved.
func (t *Template) OnDisk() bool {
	return !t.isNew
}

// AddTiles hands tiles to t as they currently look. Returns the tiles that weren't t's already, for undo.
func (l *Layer) AddTiles(t *Template, coords []util.Point) ([]util.Point, error) {
	var added []util.Point
	for _, coord := range coords {
		if coord.Z != t.Z {
			return nil, fmt.Errorf("%s lives on z %d", t.Name, t.Z)
		}
		if t.Owns(coord) {
			continue
		}
		if other := l.Owner(coord); other != nil {
			return nil, fmt.Errorf("%v already belongs to %s", coord, other.Name)
		}
		added = append(added, coord)
	}
	for _, coord := range added {
		t.footprint[coord] = &footprintTile{}
	}
	if len(added) > 0 {
		t.footprintChanged = true
	}
	return added, nil
}

// RemovedTiles is what RemoveTiles took away, so RestoreTiles can put it back exactly.
type RemovedTiles map[util.Point]*footprintTile

// RemoveTiles hands tiles back to the station map as they currently look.
func (l *Layer) RemoveTiles(t *Template, coords []util.Point) (RemovedTiles, error) {
	removed := make(RemovedTiles)
	for _, coord := range coords {
		if !t.Owns(coord) {
			continue
		}
		if l.IsFrozen(coord) {
			return nil, fmt.Errorf("%v is covered by two templates and can't change owner", coord)
		}
		removed[coord] = t.footprint[coord]
	}
	for coord := range removed {
		delete(t.footprint, coord)
	}
	if len(removed) > 0 {
		t.footprintChanged = true
	}
	return removed, nil
}

// RestoreTiles and DropTiles skip the checks, they're for undo/redo replaying a change that already passed them.

func (l *Layer) RestoreTiles(t *Template, removed RemovedTiles) {
	for coord, ft := range removed {
		t.footprint[coord] = ft
	}
	t.footprintChanged = true
}

func (l *Layer) DropTiles(t *Template, coords []util.Point) RemovedTiles {
	dropped := make(RemovedTiles, len(coords))
	for _, coord := range coords {
		dropped[coord] = t.footprint[coord]
		delete(t.footprint, coord)
	}
	t.footprintChanged = true
	return dropped
}

func isPureNoop(prefabs dmmdata.Prefabs) bool {
	return len(movablesOf(prefabs)) == 0 && isNoop(turfsOf(prefabs), noopTurf) && isNoop(areasOf(prefabs), noopArea)
}

func isNoop(prefabs dmmdata.Prefabs, noopPath string) bool {
	return len(prefabs) == 1 && prefabs[0].Path() == noopPath
}

func movablesOf(prefabs dmmdata.Prefabs) dmmdata.Prefabs {
	return filterPrefabs(prefabs, func(path string) bool {
		return !dm.IsPath(path, "/turf") && !dm.IsPath(path, "/area")
	})
}

func turfsOf(prefabs dmmdata.Prefabs) dmmdata.Prefabs {
	return filterPrefabs(prefabs, func(path string) bool { return dm.IsPath(path, "/turf") })
}

func areasOf(prefabs dmmdata.Prefabs) dmmdata.Prefabs {
	return filterPrefabs(prefabs, func(path string) bool { return dm.IsPath(path, "/area") })
}

func filterPrefabs(prefabs dmmdata.Prefabs, keep func(path string) bool) dmmdata.Prefabs {
	result := make(dmmdata.Prefabs, 0, len(prefabs))
	for _, prefab := range prefabs {
		if keep(prefab.Path()) {
			result = append(result, prefab)
		}
	}
	return result
}

// sameTile compares without building a Prefabs slice, since the panel asks every frame.
func sameTile(instances dmmap.Instances, prefabs dmmdata.Prefabs) bool {
	if len(instances) != len(prefabs) {
		return false
	}
	for idx, instance := range instances {
		if instance.Prefab().Id() != prefabs[idx].Id() {
			return false
		}
	}
	return true
}
