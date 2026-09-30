package automap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmmsave"
	"sdmm/internal/util"
)

// Save splits the live map back up: footprint tiles go to their template files, the rest to the station map.
// Templates are written first and the station last, and nothing about what counts as "saved" moves
// until all of it worked, so a failed save leaves the next attempt writing the same thing again.
func (l *Layer) Save(dme *dmenv.Dme, live *dmmap.Dmm, cfg dmmsave.Config) error {
	for _, t := range l.Templates {
		for coord, ft := range t.footprint {
			if l.IsFrozen(coord) && !sameTile(live.GetTile(coord).Instances(), ft.stamped) {
				return fmt.Errorf("tile %d,%d,%d is covered by two templates, so an edit there can't be saved back; undo it first", coord.X, coord.Y, coord.Z)
			}
		}
	}

	for _, t := range l.Templates {
		if err := l.saveTemplate(dme, live, t, cfg.SanitizeVariables); err != nil {
			return fmt.Errorf("template %s: %w", t.Name, err)
		}
	}

	station := live.Copy()
	for _, t := range l.Templates {
		for coord := range t.footprint {
			station.GetTile(coord).InstancesSet(l.base.GetTile(coord).Instances().Prefabs())
		}
	}
	if err := dmmsave.Save(dme, &station, cfg); err != nil {
		return err
	}

	l.base = &station
	for _, t := range l.Templates {
		for coord, ft := range t.footprint {
			ft.source = l.tileToWrite(live, coord, ft)
			ft.stamped = live.GetTile(coord).Instances().Prefabs()
		}
		t.footprintChanged = false
	}
	return nil
}

func (l *Layer) saveTemplate(dme *dmenv.Dme, live *dmmap.Dmm, t *Template, sanitize bool) error {
	if t.Hidden() || !t.Dirty(l, live) {
		return nil
	}

	if t.PendingDelete() {
		if err := os.Remove(t.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := l.cfg.Remove(t.Name); err != nil {
			return err
		}
		// gone from disk, so if its tiles come back (redo) it gets written out fresh
		t.isNew = true
		return nil
	}

	lo, hi := t.Bounds()
	width, height := hi.X-lo.X+1, hi.Y-lo.Y+1
	noop := dmmdata.Prefabs{dmmap.PrefabStorage.Initial(noopTurf), dmmap.PrefabStorage.Initial(noopArea)}

	if t.isNew {
		if err := writeSeed(t.path, width, height, noop); err != nil {
			return err
		}
	}

	file := &dmmap.Dmm{Name: filepath.Base(t.path), Path: dmmap.DmmPath{Absolute: t.path}, Backup: t.path}
	file.SetMapSize(width, height, 1)
	for y := 1; y <= height; y++ {
		for x := 1; x <= width; x++ {
			coord := util.Point{X: lo.X + x - 1, Y: lo.Y + y - 1, Z: t.Z}
			prefabs := noop
			if ft, ok := t.footprint[coord]; ok {
				prefabs = l.tileToWrite(live, coord, ft)
			}
			file.GetTile(util.Point{X: x, Y: y, Z: 1}).InstancesSet(prefabs)
		}
	}

	err := dmmsave.SaveV(dme, file, t.path, dmmsave.Config{Format: dmmsave.FormatTGM, SanitizeVariables: sanitize})
	if err != nil {
		if t.isNew {
			_ = os.Remove(t.path) // don't leave the seed stub where a real template should be
		}
		return err
	}

	coordinates := [3]int{lo.X, lo.Y, t.Z}
	if t.isNew {
		err = l.cfg.Append(&Entry{
			Name:        t.Name,
			MapFiles:    []string{filepath.Base(t.path)},
			Directory:   t.Directory,
			RequiredMap: l.cfg.requiredMapOf(live),
			Coordinates: coordinates[:],
			TraitName:   t.Trait,
		})
		if err != nil {
			return err
		}
		// flipped right away, so a failure further down can't get the entry appended twice
		t.isNew = false
	} else if lo.X != t.origin.X || lo.Y != t.origin.Y {
		if err = l.cfg.SetCoordinates(t.Name, coordinates); err != nil {
			return err
		}
	}
	t.origin = util.Point{X: lo.X, Y: lo.Y, Z: t.Z}
	return nil
}

// tileToWrite keeps the file's own text for untouched tiles, so a save doesn't churn them, and
// takes the live tile for edited ones. An edited overlay tile becomes a full tile, like --extract does.
func (l *Layer) tileToWrite(live *dmmap.Dmm, coord util.Point, ft *footprintTile) dmmdata.Prefabs {
	if ft.source != nil && (l.IsFrozen(coord) || sameTile(live.GetTile(coord).Instances(), ft.stamped)) {
		return ft.source
	}
	return live.GetTile(coord).Instances().Prefabs()
}

// writeSeed puts a noop-filled file where a new template goes, since dmmsave wants an existing file to reuse keys from.
func writeSeed(path string, width, height int, noop dmmdata.Prefabs) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	seed := &dmmdata.DmmData{
		Filepath:   path,
		IsTgm:      true,
		LineBreak:  "\n",
		MaxX:       width,
		MaxY:       height,
		MaxZ:       1,
		Dictionary: dmmdata.DataDictionary{"a": noop},
		Grid:       make(dmmdata.DataGrid),
	}
	for y := 1; y <= height; y++ {
		for x := 1; x <= width; x++ {
			seed.Grid[util.Point{X: x, Y: y, Z: 1}] = "a"
		}
	}
	return seed.Save()
}
