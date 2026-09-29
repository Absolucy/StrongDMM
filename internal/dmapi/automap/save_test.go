package automap

import (
	"os"
	"path/filepath"
	"testing"

	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmsave"
	"sdmm/internal/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var stationTile = []string{"/obj/pipe", "/turf/floor", "/area/hall"}

func editTemplate() testTemplate {
	return testTemplate{
		name:   "edit",
		coords: "2, 1, 1",
		tiles: []string{
			"/obj/table,\n/turf/wall,\n/area/room",
			"/obj/button,\n/turf/template_noop,\n/area/template_noop",
		},
	}
}

func (p *testProject) templatePath(name string) string {
	return filepath.Join(p.root, filepath.FromSlash(testTemplatesDir), name+".dmm")
}

func addLamp(dmm *dmmap.Dmm, x int) {
	dmm.GetTile(util.Point{X: x, Y: 1, Z: 1}).InstancesAdd(dmmap.PrefabStorage.Initial("/obj/lamp"))
}

func saveLayer(p *testProject, layer *Layer, dmm *dmmap.Dmm) error {
	return layer.Save(p.dme, dmm, dmmsave.Config{Format: dmmsave.FormatInitial})
}

func TestSave_NoEditsLeavesTemplatesAlone(t *testing.T) {
	p := newTestProject(t, editTemplate())
	before, err := os.ReadFile(p.templatePath("edit"))
	require.NoError(t, err)

	dmm, layer := p.open(t)
	require.NoError(t, saveLayer(p, layer, dmm))

	after, err := os.ReadFile(p.templatePath("edit"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "an untouched template isn't rewritten")

	station := readMap(t, p.dme, p.station)
	for x := 1; x <= 5; x++ {
		assert.Equal(t, stationTile, tilePaths(station, x), "the station keeps its own tile %d under the template", x)
	}
}

func TestSave_EditedOverlayBecomesFullTile(t *testing.T) {
	p := newTestProject(t, editTemplate())
	dmm, layer := p.open(t)

	addLamp(dmm, 3)
	require.NoError(t, saveLayer(p, layer, dmm))

	template := readMap(t, p.dme, p.templatePath("edit"))
	assert.Equal(t, []string{"/obj/table", "/turf/wall", "/area/room"}, tilePaths(template, 1), "untouched tile keeps its text")
	assert.Equal(t, []string{"/obj/pipe", "/obj/button", "/obj/lamp", "/turf/floor", "/area/hall"}, tilePaths(template, 2))

	station := readMap(t, p.dme, p.station)
	assert.Equal(t, stationTile, tilePaths(station, 3), "the edit went to the template, not the station")
}

func TestSave_EditOnOverlapIsRefused(t *testing.T) {
	p := newTestProject(t,
		testTemplate{name: "first", coords: "2, 1, 1", tiles: []string{"/obj/table,\n/turf/wall,\n/area/room"}},
		testTemplate{name: "second", coords: "2, 1, 1", tiles: []string{"/obj/lamp,\n/turf/carpet,\n/area/office"}},
	)
	stationBefore, err := os.ReadFile(p.station)
	require.NoError(t, err)
	firstBefore, err := os.ReadFile(p.templatePath("first"))
	require.NoError(t, err)

	dmm, layer := p.open(t)
	assert.True(t, layer.IsFrozen(util.Point{X: 2, Y: 1, Z: 1}))
	addLamp(dmm, 2)

	assert.Error(t, saveLayer(p, layer, dmm))

	stationAfter, _ := os.ReadFile(p.station)
	firstAfter, _ := os.ReadFile(p.templatePath("first"))
	assert.Equal(t, string(stationBefore), string(stationAfter), "nothing gets written")
	assert.Equal(t, string(firstBefore), string(firstAfter), "nothing gets written")
}

func TestSave_NewTemplateFromEdit(t *testing.T) {
	p := newTestProject(t)
	dmm, layer := p.open(t)

	// the usual workflow: edit the station in place, then hand the edited tile to a new template
	addLamp(dmm, 5)
	_, _, err := layer.NewTemplate("station_edit", "Station", dmm.Name, 1, []util.Point{{X: 5, Y: 1, Z: 1}})
	require.NoError(t, err)
	require.NoError(t, saveLayer(p, layer, dmm))

	template := readMap(t, p.dme, p.templatePath("station_edit"))
	assert.Equal(t, 1, template.MaxX)
	assert.Equal(t, []string{"/obj/pipe", "/obj/lamp", "/turf/floor", "/area/hall"}, tilePaths(template, 1))

	cfg, err := FindConfig(p.root)
	require.NoError(t, err)
	entry := cfg.Entry("station_edit")
	require.NotNil(t, entry)
	assert.Equal(t, []int{5, 1, 1}, entry.Coordinates)
	assert.Equal(t, testTemplatesDir, entry.Directory)
	assert.Equal(t, "station.dmm", entry.RequiredMap)

	station := readMap(t, p.dme, p.station)
	assert.Equal(t, stationTile, tilePaths(station, 5), "the station gets its disk version back under the new template")
}

func TestSave_EmptiedTemplateIsDeleted(t *testing.T) {
	p := newTestProject(t, editTemplate())
	dmm, layer := p.open(t)

	template := layer.Templates[0]
	_, err := layer.RemoveTiles(template, template.Coords())
	require.NoError(t, err)
	require.True(t, template.PendingDelete())
	require.NoError(t, saveLayer(p, layer, dmm))

	_, err = os.Stat(p.templatePath("edit"))
	assert.ErrorIs(t, err, os.ErrNotExist)
	cfg, err := FindConfig(p.root)
	require.NoError(t, err)
	assert.Nil(t, cfg.Entry("edit"))

	// the tiles now belong to the station, looking the way they did when they were handed over
	station := readMap(t, p.dme, p.station)
	assert.Equal(t, []string{"/obj/table", "/turf/wall", "/area/room"}, tilePaths(station, 2))
}
