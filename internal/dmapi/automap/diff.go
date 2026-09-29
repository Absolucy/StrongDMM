package automap

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/dmapi/dmmap/dmmdata"
	"sdmm/internal/util"
)

// Same knobs automapper-gen defaults to.
const (
	editGap     = 3 // edits this close (Chebyshev) share a template
	editPadding = 1 // unchanged neighbors this far out ride along
)

// NewTemplatesFromEdits turns every edit against the station map's git HEAD into templates, the way
// automapper-gen does it: changed tiles get grouped, padded, and each group becomes one template.
// The station file goes back to its HEAD content under them on save.
func (l *Layer) NewTemplatesFromEdits(dme *dmenv.Dme, live *dmmap.Dmm, baseName, trait string) ([]*Template, error) {
	head, err := loadHead(dme, live)
	if err != nil {
		return nil, err
	}
	if head.MaxX != live.MaxX || head.MaxY != live.MaxY || head.MaxZ != live.MaxZ {
		return nil, fmt.Errorf("the map is %dx%dx%d but HEAD's is %dx%dx%d, can't diff them", live.MaxX, live.MaxY, live.MaxZ, head.MaxX, head.MaxY, head.MaxZ)
	}

	var created []*Template
	for z := 1; z <= live.MaxZ; z++ {
		changes := l.changedTiles(head, live, z)
		if len(changes) == 0 {
			continue
		}
		adjacency := areaAdjacency(head, z)
		for area, neighbors := range areaAdjacency(live, z) {
			for neighbor := range neighbors {
				addAdjacent(adjacency, area, neighbor)
			}
		}

		for _, group := range clusterEdits(changes, head, live, adjacency) {
			name := baseName
			for n := 2; l.NameTaken(name); n++ {
				name = fmt.Sprintf("%s_%d", baseName, n)
			}
			t, _, err := l.NewTemplate(name, trait, live.Name, z, l.padded(live, group))
			if err != nil {
				// the ones already made are real, hand them back so the caller can still undo them
				return created, err
			}
			// like automapper-gen's revert: the edit lives in the template now, the station goes back to HEAD
			for coord := range t.footprint {
				l.base.GetTile(coord).InstancesSet(head.GetTile(coord).Instances().Prefabs())
			}
			created = append(created, t)
		}
	}
	return created, nil
}

func loadHead(dme *dmenv.Dme, live *dmmap.Dmm) (*dmmap.Dmm, error) {
	dir, file := filepath.Split(live.Path.Absolute)
	cmd := exec.Command("git", "-C", dir, "show", "HEAD:./"+file)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("git show HEAD:%s: %s", file, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("running git: %w", err)
	}

	// the parser only reads files
	tmp, err := os.CreateTemp("", "sdmm-head-*.dmm")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	_, err = tmp.Write(out)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}

	data, err := dmmdata.New(tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("parsing HEAD's %s: %w", file, err)
	}
	head, _ := dmmap.New(dme, data, "")
	return head, nil
}

// changedTiles skips tiles a template already owns, those edits have a home.
func (l *Layer) changedTiles(head, live *dmmap.Dmm, z int) []util.Point {
	var changes []util.Point
	for y := 1; y <= live.MaxY; y++ {
		for x := 1; x <= live.MaxX; x++ {
			coord := util.Point{X: x, Y: y, Z: z}
			if l.Owner(coord) != nil {
				continue
			}
			if !sameContent(head.GetTile(coord).Instances().Prefabs(), live.GetTile(coord).Instances().Prefabs()) {
				changes = append(changes, coord)
			}
		}
	}
	return changes
}

// clusterEdits groups edits that are within editGap of each other, or whose areas are the same or
// border each other anywhere on the level, however far apart the tiles are.
func clusterEdits(changes []util.Point, head, live *dmmap.Dmm, adjacency map[string]map[string]bool) [][]util.Point {
	parent := make([]int, len(changes))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}

	areas := make([]map[string]bool, len(changes))
	for i, coord := range changes {
		areas[i] = map[string]bool{}
		for _, dmm := range []*dmmap.Dmm{head, live} {
			if area := areaOf(dmm.GetTile(coord).Instances().Prefabs()); area != "" {
				areas[i][area] = true
			}
		}
	}

	for i, a := range changes {
		for j := i + 1; j < len(changes); j++ {
			b := changes[j]
			if max(abs(a.X-b.X), abs(a.Y-b.Y)) <= editGap || areasRelated(areas[i], areas[j], adjacency) {
				parent[find(i)] = find(j)
			}
		}
	}

	groups := map[int][]util.Point{}
	var order []int
	for i, coord := range changes {
		root := find(i)
		if _, ok := groups[root]; !ok {
			order = append(order, root)
		}
		groups[root] = append(groups[root], coord)
	}
	result := make([][]util.Point, 0, len(order))
	for _, root := range order {
		result = append(result, groups[root])
	}
	return result
}

func areasRelated(a, b map[string]bool, adjacency map[string]map[string]bool) bool {
	for area := range a {
		if b[area] {
			return true
		}
		for other := range b {
			if adjacency[area][other] {
				return true
			}
		}
	}
	return false
}

// padded takes the group plus its neighbors within editPadding, copied as they are, so an unchanged
// wall in the middle of an edit belongs to the template too. Tiles other templates own stay theirs.
func (l *Layer) padded(live *dmmap.Dmm, group []util.Point) []util.Point {
	seen := map[util.Point]bool{}
	var positions []util.Point
	for _, coord := range group {
		for dy := -editPadding; dy <= editPadding; dy++ {
			for dx := -editPadding; dx <= editPadding; dx++ {
				p := util.Point{X: coord.X + dx, Y: coord.Y + dy, Z: coord.Z}
				if seen[p] || !live.HasTile(p) || l.Owner(p) != nil {
					continue
				}
				seen[p] = true
				positions = append(positions, p)
			}
		}
	}
	return positions
}

// areaAdjacency maps which areas border which on a level. Right/up per tile covers every border once.
func areaAdjacency(dmm *dmmap.Dmm, z int) map[string]map[string]bool {
	adjacency := map[string]map[string]bool{}
	for y := 1; y <= dmm.MaxY; y++ {
		for x := 1; x <= dmm.MaxX; x++ {
			area := areaOf(dmm.GetTile(util.Point{X: x, Y: y, Z: z}).Instances().Prefabs())
			if area == "" {
				continue
			}
			for _, n := range []util.Point{{X: x + 1, Y: y, Z: z}, {X: x, Y: y + 1, Z: z}} {
				if !dmm.HasTile(n) {
					continue
				}
				if neighbor := areaOf(dmm.GetTile(n).Instances().Prefabs()); neighbor != "" && neighbor != area {
					addAdjacent(adjacency, area, neighbor)
				}
			}
		}
	}
	return adjacency
}

func addAdjacent(adjacency map[string]map[string]bool, a, b string) {
	if adjacency[a] == nil {
		adjacency[a] = map[string]bool{}
	}
	if adjacency[b] == nil {
		adjacency[b] = map[string]bool{}
	}
	adjacency[a][b] = true
	adjacency[b][a] = true
}

func areaOf(prefabs dmmdata.Prefabs) string {
	for idx := len(prefabs) - 1; idx >= 0; idx-- {
		if dm.IsPath(prefabs[idx].Path(), "/area") {
			return prefabs[idx].Path()
		}
	}
	return ""
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

var (
	quotedString  = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)
	numberLiteral = regexp.MustCompile(`-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?`)
	whitespace    = regexp.MustCompile(`\s+`)
)

// sameContent compares tiles the way automapper-gen does: a map resave that only changed spacing or
// number notation (1 vs 1.0) on a var isn't an edit. Quoted strings are compared as-is.
func sameContent(a, b dmmdata.Prefabs) bool {
	if a.Equals(b) {
		return true // nearly every tile, and way cheaper than the regexes
	}
	if len(a) != len(b) {
		return false
	}
	for idx := range a {
		if a[idx].Path() != b[idx].Path() {
			return false
		}
		aNames, bNames := a[idx].Vars().Iterate(), b[idx].Vars().Iterate()
		if len(aNames) != len(bNames) {
			return false
		}
		for i, name := range aNames {
			if bNames[i] != name {
				return false
			}
			aValue, _ := a[idx].Vars().Value(name)
			bValue, _ := b[idx].Vars().Value(name)
			if normalizeValue(aValue) != normalizeValue(bValue) {
				return false
			}
		}
	}
	return true
}

func normalizeValue(value string) string {
	var sb strings.Builder
	pos := 0
	for _, loc := range quotedString.FindAllStringIndex(value, -1) {
		sb.WriteString(normalizeUnquoted(value[pos:loc[0]]))
		sb.WriteString(value[loc[0]:loc[1]])
		pos = loc[1]
	}
	sb.WriteString(normalizeUnquoted(value[pos:]))
	return sb.String()
}

// The boundary checks stand in for lookarounds, which Go's regexp doesn't have: a number glued to
// a word (icon_state1, /obj/x2) is part of that word, not a number.
func normalizeUnquoted(text string) string {
	text = whitespace.ReplaceAllString(text, "")
	var sb strings.Builder
	pos := 0
	for _, loc := range numberLiteral.FindAllStringIndex(text, -1) {
		if loc[0] > 0 && (isWordByte(text[loc[0]-1]) || text[loc[0]-1] == '.') || loc[1] < len(text) && isWordByte(text[loc[1]]) {
			continue
		}
		number, err := strconv.ParseFloat(text[loc[0]:loc[1]], 64)
		if err != nil {
			continue
		}
		sb.WriteString(text[pos:loc[0]])
		sb.WriteString(strconv.FormatFloat(number, 'g', -1, 64))
		pos = loc[1]
	}
	sb.WriteString(text[pos:])
	return sb.String()
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
