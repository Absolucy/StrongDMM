package automap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleConfig = `# Automapper configuration
# coordinates - blah

# Metastation Arrivals
[templates.metastation_arrivals]
map_files = ["metastation_arrivals.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [20, 107, 1]
trait_name = "Station"

# Metastation Cryo
[templates.metastation_cryo]
map_files = ["metastation_cryo.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [133, 182, 1]
trait_name = "Station"

# Metastation Barber
[templates.metastation_barber]
map_files = ["metastation_barber.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [101, 116, 1]
trait_name = "Station"
`

func TestSetCoordinates(t *testing.T) {
	got, err := setCoordinates(sampleConfig, "metastation_cryo", [3]int{1, 2, 3})
	require.NoError(t, err)

	want := `# Metastation Cryo
[templates.metastation_cryo]
map_files = ["metastation_cryo.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [1, 2, 3]
trait_name = "Station"`
	assert.Contains(t, got, want)
	assert.Contains(t, got, "coordinates = [20, 107, 1]", "other entries keep their coordinates")
	assert.Contains(t, got, "coordinates = [101, 116, 1]", "other entries keep their coordinates")
	assert.Len(t, got, len(sampleConfig)-len("133, 182, 1")+len("1, 2, 3"))

	_, err = setCoordinates(sampleConfig, "metastation", [3]int{1, 2, 3})
	assert.Error(t, err, "a prefix of a real name isn't a match")
}

func TestAppendEntry(t *testing.T) {
	got := appendEntry(sampleConfig, &Entry{
		Name:        "metastation_edit",
		MapFiles:    []string{"metastation_edit.dmm"},
		Directory:   "_maps/oculis/automapper/templates/metastation/",
		RequiredMap: "MetaStation.dmm",
		Coordinates: []int{5, 6, 1},
		TraitName:   "Station",
	})

	assert.Equal(t, sampleConfig+`
# metastation_edit
[templates.metastation_edit]
map_files = ["metastation_edit.dmm"]
directory = "_maps/oculis/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [5, 6, 1]
trait_name = "Station"
`, got)
}

func TestRemoveEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		want  string
	}{
		{
			name:  "middle entry takes its comment, the next entry keeps its own",
			entry: "metastation_cryo",
			want: `# Automapper configuration
# coordinates - blah

# Metastation Arrivals
[templates.metastation_arrivals]
map_files = ["metastation_arrivals.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [20, 107, 1]
trait_name = "Station"

# Metastation Barber
[templates.metastation_barber]
map_files = ["metastation_barber.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [101, 116, 1]
trait_name = "Station"
`,
		},
		{
			name:  "last entry",
			entry: "metastation_barber",
			want: `# Automapper configuration
# coordinates - blah

# Metastation Arrivals
[templates.metastation_arrivals]
map_files = ["metastation_arrivals.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [20, 107, 1]
trait_name = "Station"

# Metastation Cryo
[templates.metastation_cryo]
map_files = ["metastation_cryo.dmm"]
directory = "_maps/nova/automapper/templates/metastation/"
required_map = "MetaStation.dmm"
coordinates = [133, 182, 1]
trait_name = "Station"
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := removeEntry(sampleConfig, tt.entry)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
