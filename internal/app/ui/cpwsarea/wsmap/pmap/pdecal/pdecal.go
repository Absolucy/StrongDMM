package pdecal

import (
	"sort"
	"strings"

	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmicon"
	"sdmm/internal/dmapi/dmmap/dmmdata/dmmprefab"
	"sdmm/internal/dmapi/dmvars"
	"sdmm/internal/imguiext/style"
	w "sdmm/internal/imguiext/widget"
	"sdmm/internal/util"

	"github.com/SpaiR/imgui-go"
)

const (
	gridColumns = 8
	gridRows    = 10
)

// Decals are mostly half-transparent, so they need something floor-like behind to be seen.
var floorColor = imgui.Vec4{X: .4, Y: .41, Z: .42, W: 1}

type App interface {
	LoadedEnvironment() *dmenv.Dme
	SelectedPrefab() (*dmmprefab.Prefab, bool)
	DoSelectPrefabByPath(string)
}

type decal struct {
	path   string
	sprite *dmicon.Sprite
	tint   imgui.Vec4
	shown  bool
}

// Decals with the same first part of the path after the root: "tile", "trimline", "stripes" etc.
type group struct {
	name   string
	decals []*decal
	shown  int
}

// Panel is a grid of all decal types the environment has, to select them by their look.
type Panel struct {
	app App

	filter string
	groups []*group
	// The environment the groups are made for.
	environment *dmenv.Dme
}

func New(app App) *Panel {
	return &Panel{app: app}
}

func (p *Panel) Process() {
	if dme := p.app.LoadedEnvironment(); dme != p.environment {
		p.environment = dme
		p.groups = makeGroups(dme)
		p.doFilter()
	}

	if len(p.groups) == 0 {
		imgui.TextDisabled("No " + tools.DecalRoot + " types in the environment")
		return
	}

	iconSize := 32 * window.PointSize()
	padding := imgui.CurrentStyle().FramePadding()
	spacing := imgui.CurrentStyle().ItemSpacing()
	cellWidth := iconSize + padding.X*2 + spacing.X
	cellHeight := iconSize + padding.Y*2 + spacing.Y
	width := cellWidth*gridColumns + imgui.CurrentStyle().ScrollbarSize()

	w.InputTextWithHint("##decal_filter", "Search", &p.filter).
		ButtonClear().
		Width(width).
		OnChange(p.doFilter).
		Build()

	if imgui.BeginChildV("decal_grid", imgui.Vec2{X: width, Y: cellHeight * gridRows}, false, imgui.WindowFlagsNone) {
		for _, group := range p.groups {
			p.showGroup(group, iconSize)
		}
	}
	imgui.EndChild()
}

func (p *Panel) showGroup(group *group, iconSize float32) {
	if group.shown == 0 || !imgui.CollapsingHeaderV(group.name, imgui.TreeNodeFlagsDefaultOpen) {
		return
	}

	var selectedPath string
	if prefab, ok := p.app.SelectedPrefab(); ok {
		selectedPath = prefab.Path()
	}

	imgui.PushID(group.name)
	column := 0
	for idx, decal := range group.decals {
		if !decal.shown {
			continue
		}
		if column%gridColumns != 0 {
			imgui.SameLine()
		}
		column++

		s := decal.sprite
		imgui.PushIDInt(idx) // all buttons of the same .dmi have the same texture, which is their id
		clicked := imgui.ImageButtonV(
			imgui.TextureID(s.Texture()),
			imgui.Vec2{X: iconSize, Y: iconSize},
			imgui.Vec2{X: s.U1, Y: s.V1}, imgui.Vec2{X: s.U2, Y: s.V2},
			-1, floorColor, decal.tint,
		)
		imgui.PopID()

		if clicked {
			p.app.DoSelectPrefabByPath(decal.path)
		}
		if imgui.IsItemHovered() {
			imgui.SetTooltip(decal.path)
		}
		if decal.path == selectedPath {
			imgui.WindowDrawList().AddRectV(
				imgui.ItemRectMin(), imgui.ItemRectMax(),
				imgui.PackedColorFromVec4(style.ColorGreen3), 0, imgui.DrawFlagsNone, 2*window.PointSize(),
			)
		}
	}
	imgui.PopID()
}

// Every word of the filter must be somewhere in the path.
func (p *Panel) doFilter() {
	words := strings.Fields(strings.ToLower(p.filter))
	for _, group := range p.groups {
		group.shown = 0
		for _, decal := range group.decals {
			decal.shown = true
			for _, word := range words {
				if !strings.Contains(decal.path, word) {
					decal.shown = false
					break
				}
			}
			if decal.shown {
				group.shown++
			}
		}
	}
}

// Tint returns the color to draw the sprite with.
// The same rule the map render has: alpha counts only together with color.
func Tint(vars *dmvars.Variables) imgui.Vec4 {
	if color, _ := vars.Text("color"); color != "" {
		r, g, b, _ := util.ParseColor(color).RGBA()
		return imgui.Vec4{X: r, Y: g, Z: b, W: vars.FloatV("alpha", 255) / 255}
	}
	return imgui.Vec4{X: 1, Y: 1, Z: 1, W: 1}
}

func makeGroups(dme *dmenv.Dme) []*group {
	if dme == nil || dme.Objects[tools.DecalRoot] == nil {
		return nil
	}

	var paths []string
	var collect func(object *dmenv.Object)
	collect = func(object *dmenv.Object) {
		for _, child := range object.DirectChildren {
			paths = append(paths, child)
			collect(dme.Objects[child])
		}
	}
	collect(dme.Objects[tools.DecalRoot])
	sort.Strings(paths)

	var groups []*group
	byName := make(map[string]*group)
	for _, path := range paths {
		name, _, _ := strings.Cut(strings.TrimPrefix(path, tools.DecalRoot+"/"), "/")
		if byName[name] == nil {
			byName[name] = &group{name: name}
			groups = append(groups, byName[name])
		}

		vars := dme.Objects[path].Vars
		icon, _ := vars.Text("icon")
		iconState, _ := vars.Text("icon_state")
		dir, _ := vars.Int("dir")
		byName[name].decals = append(byName[name].decals, &decal{
			path:   path,
			sprite: dmicon.Cache.GetSpriteOrPlaceholderV(icon, iconState, dir),
			tint:   Tint(vars),
		})
	}
	return groups
}
