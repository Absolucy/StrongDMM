package automap

import (
	"os/exec"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a 16x1 station: hall on 1-8, space on 9-12, room on 13-16. Hall and room don't touch.
func diffStation() []string {
	var tiles []string
	for x := 1; x <= 16; x++ {
		switch {
		case x <= 8:
			tiles = append(tiles, "/turf/floor,\n/area/hall")
		case x <= 12:
			tiles = append(tiles, "/turf/floor,\n/area/space")
		default:
			tiles = append(tiles, "/turf/floor,\n/area/room")
		}
	}
	return tiles
}

func gitCommitAll(t *testing.T, root string) {
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "."},
		{"-c", "user.name=test", "-c", "user.email=test@test", "-c", "commit.gpgsign=false", "commit", "-qm", "base"},
	} {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
}

func footprintXs(tmpl *Template) []int {
	var xs []int
	for _, coord := range tmpl.Coords() {
		xs = append(xs, coord.X)
	}
	sort.Ints(xs)
	return xs
}

func TestNewTemplatesFromEdits_Grouping(t *testing.T) {
	tests := []struct {
		name  string
		edits []int
		want  [][]int // footprint x's per template, padding included
	}{
		{name: "same area far apart stays together", edits: []int{1, 8}, want: [][]int{{1, 2, 7, 8, 9}}},
		{name: "unrelated areas far apart split", edits: []int{1, 16}, want: [][]int{{1, 2}, {15, 16}}},
		{name: "bordering areas stay together", edits: []int{10, 14}, want: [][]int{{9, 10, 11, 13, 14, 15}}},
		{name: "close edits stay together whatever the areas", edits: []int{6, 9}, want: [][]int{{5, 6, 7, 8, 9, 10}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newTestProject(t)
			writeTGM(t, p.station, diffStation())
			gitCommitAll(t, p.root)
			dmm, layer := p.open(t)

			for _, x := range tt.edits {
				addLamp(dmm, x)
			}
			created, err := layer.NewTemplatesFromEdits(p.dme, dmm, "station_edit", "Station")
			require.NoError(t, err)

			require.Len(t, created, len(tt.want))
			for idx, want := range tt.want {
				assert.Equal(t, want, footprintXs(created[idx]), "template %d", idx)
			}
			assert.Equal(t, "station_edit", created[0].Name)
			if len(created) > 1 {
				assert.Equal(t, "station_edit_2", created[1].Name)
			}
		})
	}
}

func TestNewTemplatesFromEdits_SkipsTemplatesAndFormatting(t *testing.T) {
	p := newTestProject(t, testTemplate{name: "existing", coords: "5, 1, 1", tiles: []string{"/obj/table,\n/turf/wall,\n/area/room"}})
	tiles := diffStation()
	tiles[1] = "/obj/pipe{\n\tpixel_x = 1\n\t},\n/turf/floor,\n/area/hall"
	writeTGM(t, p.station, tiles)
	gitCommitAll(t, p.root)

	// a resave that only rewrote how the number looks
	tiles[1] = "/obj/pipe{\n\tpixel_x = 1.0\n\t},\n/turf/floor,\n/area/hall"
	writeTGM(t, p.station, tiles)

	dmm, layer := p.open(t)
	addLamp(dmm, 5) // inside the existing template, so it already has a home

	created, err := layer.NewTemplatesFromEdits(p.dme, dmm, "station_edit", "Station")
	require.NoError(t, err)
	assert.Empty(t, created)
}

func TestNewTemplatesFromEdits_StationRevertsToHead(t *testing.T) {
	p := newTestProject(t)
	tiles := diffStation()
	writeTGM(t, p.station, tiles)
	gitCommitAll(t, p.root)

	// the edit was already saved into the station file, which is the case git HEAD is for
	tiles[0] = "/obj/lamp,\n/turf/floor,\n/area/hall"
	writeTGM(t, p.station, tiles)

	dmm, layer := p.open(t)
	created, err := layer.NewTemplatesFromEdits(p.dme, dmm, "station_edit", "Station")
	require.NoError(t, err)
	require.Len(t, created, 1)
	require.NoError(t, saveLayer(p, layer, dmm))

	station := readMap(t, p.dme, p.station)
	assert.Equal(t, []string{"/turf/floor", "/area/hall"}, tilePaths(station, 1), "the station file is back to HEAD under the template")
	template := readMap(t, p.dme, p.templatePath("station_edit"))
	assert.Equal(t, []string{"/obj/lamp", "/turf/floor", "/area/hall"}, tilePaths(template, 1))
}

func TestNormalizeValue(t *testing.T) {
	tests := []struct {
		a, b string
		same bool
	}{
		{a: "1", b: "1.0", same: true},
		{a: "list(1,2)", b: "list(1.0, 2.0)", same: true},
		{a: "-32", b: "-32.00", same: true},
		{a: "1e+006", b: "1000000", same: true},
		{a: `"1.0"`, b: `"1"`, same: false},
		{a: "icon_state1", b: "icon_state1.0", same: false},
		{a: "2", b: "3", same: false},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.same, normalizeValue(tt.a) == normalizeValue(tt.b))
		})
	}
}
