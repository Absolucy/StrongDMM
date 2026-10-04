package tools

import (
	"testing"

	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmvars"

	"github.com/stretchr/testify/assert"
)

const (
	testDecalLine   = DecalRoot + "/line"
	testDecalCorner = DecalRoot + "/corner"
	testDecalSiding = DecalRoot + "/siding"
	testFloor       = "/turf/open/floor"
	testWall        = closedTurf + "/wall"
	// faces east by default, unlike everything else in BYOND
	testDecalEast = DecalRoot + "/east"
)

func testDecalEnv() {
	world := &dmvars.MutableVariables{}
	world.Put("icon_size", "32")
	world.Put("area", "/area/space")
	world.Put("turf", "/turf/space")

	east := &dmvars.MutableVariables{}
	east.Put("dir", "4")

	objects := map[string]*dmenv.Object{
		"/world":      {Path: "/world", Vars: world.ToImmutable()},
		testDecalEast: {Path: testDecalEast, Vars: east.ToImmutable()},
	}
	for _, path := range []string{
		"/area/space", "/turf/space", testFloor, testWall,
		testDecalLine, testDecalCorner, testDecalSiding, testDecalSiding + "/corner",
	} {
		objects[path] = &dmenv.Object{Path: path, Vars: &dmvars.Variables{}}
	}

	dmmap.PrefabStorage.Free()
	dmmap.Init(&dmenv.Dme{Objects: objects})
}

func TestCornerOf(t *testing.T) {
	testDecalEnv()
	initial := dmmap.PrefabStorage.Initial

	corner, ok := cornerOf(initial(testDecalLine))
	assert.True(t, ok)
	assert.Equal(t, testDecalCorner, corner.Path(), "a line has its corner next to it")

	corner, ok = cornerOf(initial(testDecalSiding))
	assert.True(t, ok)
	assert.Equal(t, testDecalSiding+"/corner", corner.Path(), "others have it as a subtype")

	_, ok = cornerOf(initial(testDecalEast))
	assert.False(t, ok)
}

func TestCanPlace(t *testing.T) {
	testDecalEnv()
	line := dmmap.PrefabStorage.Initial(testDecalLine)

	floor := &dmmap.Tile{}
	floor.InstancesAdd(dmmap.PrefabStorage.Initial(testFloor))
	assert.True(t, canPlace(floor, line))

	floor.InstancesAdd(PrefabWithDir(line, dm.DirNorth))
	assert.True(t, canPlace(floor, line), "the same decal facing another way is a different one")
	floor.InstancesAdd(line)
	assert.False(t, canPlace(floor, line))

	wall := &dmmap.Tile{}
	wall.InstancesAdd(dmmap.PrefabStorage.Initial(testWall))
	assert.False(t, canPlace(wall, line))
}

func TestBoxAims(t *testing.T) {
	assert.Equal(t, -1, boxSide(3, 7, 3))
	assert.Equal(t, 0, boxSide(3, 7, 5))
	assert.Equal(t, 1, boxSide(3, 7, 7))

	assert.Equal(t, [][2]float64{{0, 1}}, boxAims(0, 1, false), "a side faces out of the box")
	assert.Equal(t, [][2]float64{{-1, 1}}, boxAims(-1, 1, true), "a corner takes the sprite for both sides")
	assert.Equal(t, [][2]float64{{-1, 0}, {0, 1}}, boxAims(-1, 1, false), "or a decal per side, when there is none")
}

func TestPrefabWithDir(t *testing.T) {
	testDecalEnv()

	line := dmmap.PrefabStorage.Initial(testDecalLine)

	north := PrefabWithDir(line, dm.DirNorth)
	assert.Equal(t, "1", north.Vars().ValueV("dir", ""))
	assert.Same(t, line, PrefabWithDir(north, dm.DirSouth), "the default dir isn't worth a var in the map file")

	// The default is whatever the type says, not always south.
	east := dmmap.PrefabStorage.Initial(testDecalEast)
	assert.Same(t, east, PrefabWithDir(east, dm.DirEast))
	south := PrefabWithDir(east, dm.DirSouth)
	assert.Equal(t, 1, south.Vars().Len())
	assert.Equal(t, "2", south.Vars().ValueV("dir", ""))

	// Other edits of the prefab stay.
	blue := dmmap.PrefabStorage.Get(testDecalLine, dmvars.Set(line.Vars(), "color", `"#52B4E9"`))
	blueNorth := PrefabWithDir(blue, dm.DirNorth)
	assert.Equal(t, `"#52B4E9"`, blueNorth.Vars().ValueV("color", ""))
	assert.Equal(t, "1", blueNorth.Vars().ValueV("dir", ""))
}
