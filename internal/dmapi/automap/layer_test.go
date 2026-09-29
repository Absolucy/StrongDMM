package automap

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTemplatesDir = "_maps/test/automapper/templates/station/"

// testProject is a throwaway repo: a 5x1 station map, where every tile is pipe + floor + hall,
// plus an automapper config pointing at the given templates.
type testProject struct {
	root    string
	dme     *dmenv.Dme
	station string
}

type testTemplate struct {
	name   string
	coords string // "x, y, z"
	tiles  []string
}

func newTestProject(t *testing.T, templates ...testTemplate) *testProject {
	root := t.TempDir()
	p := &testProject{root: root, dme: testEnv(root), station: filepath.Join(root, "station.dmm")}

	writeTGM(t, p.station, repeat("/obj/pipe,\n/turf/floor,\n/area/hall", 5))

	config := "# test config\n"
	for _, tmpl := range templates {
		writeTGM(t, filepath.Join(root, filepath.FromSlash(testTemplatesDir), tmpl.name+".dmm"), tmpl.tiles)
		config += fmt.Sprintf(
			"\n# %s\n[templates.%s]\nmap_files = [\"%s.dmm\"]\ndirectory = \"%s\"\nrequired_map = \"station.dmm\"\ncoordinates = [%s]\ntrait_name = \"Station\"\n",
			tmpl.name, tmpl.name, tmpl.name, testTemplatesDir, tmpl.coords,
		)
	}
	writeFile(t, filepath.Join(root, "_maps", "test", "automapper", "automapper_config.toml"), config)
	return p
}

// open does what StrongDMM does when opening the station map: parse, build the Dmm, stamp.
func (p *testProject) open(t *testing.T) (*dmmap.Dmm, *Layer) {
	data, err := dmmdata.New(p.station)
	require.NoError(t, err)
	dmm, unknown := dmmap.New(p.dme, data, p.station)
	require.Empty(t, unknown)

	layer, err := Load(p.dme, dmm)
	require.NoError(t, err)
	require.NotNil(t, layer)
	return dmm, layer
}

func testEnv(root string) *dmenv.Dme {
	world := &dmvars.MutableVariables{}
	world.Put("icon_size", "32")
	world.Put("area", "/area/space")
	world.Put("turf", "/turf/space")

	objects := map[string]*dmenv.Object{"/world": {Path: "/world", Vars: world.ToImmutable()}}
	for _, path := range []string{
		"/area/space", "/turf/space", noopTurf, noopArea,
		"/obj/pipe", "/obj/table", "/obj/button", "/obj/lamp",
		"/turf/floor", "/turf/wall", "/turf/carpet",
		"/area/hall", "/area/room", "/area/office",
	} {
		objects[path] = &dmenv.Object{Path: path, Vars: &dmvars.Variables{}}
	}

	dme := &dmenv.Dme{RootDir: root, Objects: objects}
	dmmap.PrefabStorage.Free()
	dmmap.Init(dme)
	return dme
}

// writeTGM writes a one-row map, one tile per entry, each tile given as its comma-separated paths.
func writeTGM(t *testing.T, path string, tiles []string) {
	var sb strings.Builder
	sb.WriteString("//MAP CONVERTED BY dmm2tgm.py THIS HEADER COMMENT PREVENTS RECONVERSION, DO NOT REMOVE\n")
	for idx, tile := range tiles {
		fmt.Fprintf(&sb, "\"%c\" = (\n%s)\n", 'a'+idx, tile)
	}
	for idx := range tiles {
		fmt.Fprintf(&sb, "\n(%d,1,1) = {\"\n%c\n\"}", idx+1, 'a'+idx)
	}
	sb.WriteString("\n")
	writeFile(t, path, sb.String())
}

func writeFile(t *testing.T, path, content string) {
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func repeat(tile string, n int) []string {
	tiles := make([]string, n)
	for idx := range tiles {
		tiles[idx] = tile
	}
	return tiles
}

func tilePaths(dmm *dmmap.Dmm, x int) []string {
	var paths []string
	for _, prefab := range dmm.GetTile(util.Point{X: x, Y: 1, Z: 1}).Instances().Prefabs() {
		paths = append(paths, prefab.Path())
	}
	return paths
}

func readMap(t *testing.T, dme *dmenv.Dme, path string) *dmmap.Dmm {
	data, err := dmmdata.New(path)
	require.NoError(t, err)
	dmm, _ := dmmap.New(dme, data, path)
	return dmm
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		templates []testTemplate
		want      [][]string // station tiles 1..5 after stamping
	}{
		{
			name: "full, overlay, noop area on a real turf, and pure noop tiles",
			templates: []testTemplate{{
				name:   "edit",
				coords: "2, 1, 1",
				tiles: []string{
					"/obj/table,\n/turf/wall,\n/area/room",
					"/obj/button,\n/turf/template_noop,\n/area/template_noop",
					"/turf/wall,\n/area/template_noop",
					"/turf/template_noop,\n/area/template_noop",
				},
			}},
			want: [][]string{
				{"/obj/pipe", "/turf/floor", "/area/hall"},
				// the station tile is never loaded under a real template turf
				{"/obj/table", "/turf/wall", "/area/room"},
				{"/obj/pipe", "/obj/button", "/turf/floor", "/area/hall"},
				// ...so a noop area there leaves the empty level's default, not the station's area
				{"/turf/wall", "/area/space"},
				{"/obj/pipe", "/turf/floor", "/area/hall"},
			},
		},
		{
			name: "overlapping templates keep both sets of objects, turf and area from the later one",
			templates: []testTemplate{
				{name: "first", coords: "2, 1, 1", tiles: []string{"/obj/table,\n/turf/wall,\n/area/room"}},
				{name: "second", coords: "2, 1, 1", tiles: []string{"/obj/lamp,\n/turf/carpet,\n/area/office"}},
			},
			want: [][]string{
				{"/obj/pipe", "/turf/floor", "/area/hall"},
				{"/obj/table", "/obj/lamp", "/turf/carpet", "/area/office"},
				{"/obj/pipe", "/turf/floor", "/area/hall"},
				{"/obj/pipe", "/turf/floor", "/area/hall"},
				{"/obj/pipe", "/turf/floor", "/area/hall"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dmm, layer := newTestProject(t, tt.templates...).open(t)
			assert.Empty(t, layer.Warnings)
			for x, want := range tt.want {
				assert.Equal(t, want, tilePaths(dmm, x+1), "tile %d", x+1)
			}
		})
	}
}

func TestLoad_SkipsTemplateOffTheMap(t *testing.T) {
	dmm, layer := newTestProject(t, testTemplate{
		name:   "too_wide",
		coords: "4, 1, 1",
		tiles:  repeat("/obj/table,\n/turf/wall,\n/area/room", 3),
	}).open(t)

	assert.Empty(t, layer.Templates)
	require.Len(t, layer.Warnings, 1)
	assert.Contains(t, layer.Warnings[0], "too_wide")
	for x := 1; x <= 5; x++ {
		assert.Equal(t, []string{"/obj/pipe", "/turf/floor", "/area/hall"}, tilePaths(dmm, x))
	}
}
