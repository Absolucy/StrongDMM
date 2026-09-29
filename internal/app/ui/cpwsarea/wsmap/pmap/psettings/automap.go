package psettings

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"sdmm/internal/app/ui/cpwsarea/wsmap/pmap/overlay"
	"sdmm/internal/app/ui/cpwsarea/wsmap/tools"
	appdialog "sdmm/internal/app/ui/dialog"
	"sdmm/internal/dmapi/automap"
	"sdmm/internal/imguiext/style"
	"sdmm/internal/util"

	"github.com/SpaiR/imgui-go"
	"github.com/rs/zerolog/log"
)

type sessionAutomap struct {
	selected string // name of the template the add/remove buttons act on
	expand   bool


	prefilled bool
	newName   string
	newTrait  string
}

const noSelectionHint = "Select tiles with the Grab tool first."

// ExpandAutomap opens the Automapper section the next time the panel draws.
func (p *Panel) ExpandAutomap() {
	p.sessionAutomap.expand = true
}

func (p *Panel) showAutomap() {
	if p.layer == nil {
		return
	}
	if p.sessionAutomap.expand {
		imgui.SetNextItemOpen(true, imgui.ConditionAlways)
		p.sessionAutomap.expand = false
	}
	if !imgui.CollapsingHeader("Automapper") {
		return
	}

	for _, warning := range p.layer.Warnings {
		p.warningText(warning)
	}

	shown := 0
	for idx, t := range p.layer.Templates {
		if t.Hidden() {
			continue
		}
		shown++
		p.showAutomapTemplate(idx, t)
	}
	if shown == 0 {
		imgui.TextDisabled("No templates for this map yet.")
	}

	imgui.Separator()

	coords, hasSelection := p.grabSelection()
	selected := p.selectedTemplate()

	if !p.sessionAutomap.prefilled {
		p.sessionAutomap.newName = p.suggestTemplateName()
		p.sessionAutomap.newTrait = "Station"
		p.sessionAutomap.prefilled = true
	}
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##automap_name", "template name", &p.sessionAutomap.newName)
	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##automap_trait", "z-level trait", &p.sessionAutomap.newTrait)

	if imgui.ButtonV("New Templates from Edits", imgui.Vec2{X: -1}) {
		p.doNewTemplatesFromEdits()
	}
	if imgui.IsItemHovered() {
		imgui.SetTooltip("Diff the map against its git HEAD and turn every edit outside a template into templates,\ngrouped the same way automapper-gen does it. The name above is used as the base name.")
	}
	p.selectionButton("New Template from Selection", hasSelection, noSelectionHint, func() {
		p.doNewTemplate(coords)
	})

	disabledReason := noSelectionHint
	if selected == nil {
		disabledReason = "Pick a template in the list first."
	}
	canEdit := hasSelection && selected != nil
	p.selectionButton("Add Selection to Template", canEdit, disabledReason, func() {
		p.doAddTiles(selected, coords)
	})
	p.selectionButton("Remove Selection from Template", canEdit, disabledReason, func() {
		p.doRemoveTiles(selected, coords)
	})
}

func (p *Panel) showAutomapTemplate(idx int, t *automap.Template) {
	imgui.PushID(t.Name)
	defer imgui.PopID()

	r, g, b, a := overlay.ColorAutomapTemplate(idx).RGBA()
	imgui.ColorButton("##color", imgui.Vec4{X: r, Y: g, Z: b, W: a}, imgui.ColorEditFlagsNoTooltip|imgui.ColorEditFlagsNoDragDrop, imgui.Vec2{})
	imgui.SameLine()

	// the button goes before the name, a selectable eats the rest of the line
	imgui.BeginDisabledV(t.PendingDelete())
	if imgui.SmallButton("Jump") {
		lo, hi := t.Bounds()
		p.editor.FocusCameraOnPosition(util.Point{X: (lo.X + hi.X) / 2, Y: (lo.Y + hi.Y) / 2, Z: t.Z})
	}
	imgui.EndDisabled()
	imgui.SameLine()

	label := t.Name
	if t.Dirty(p.layer, p.editor.Dmm()) {
		label += " *"
	}
	if imgui.SelectableV(label+"###row", p.sessionAutomap.selected == t.Name, imgui.SelectableFlagsNone, imgui.Vec2{}) {
		p.sessionAutomap.selected = t.Name
	}
	if imgui.IsItemHovered() {
		if t.Size() == 0 {
			imgui.SetTooltip(t.Path())
		} else {
			lo, _ := t.Bounds()
			imgui.SetTooltip(fmt.Sprintf("%d tiles, bottom-left at %d,%d on z %d\n%s", t.Size(), lo.X, lo.Y, t.Z, t.Path()))
		}
	}

	if t.PendingDelete() {
		p.warningText("No tiles left, deleted on next save.")
	}
}

func (p *Panel) warningText(text string) {
	imgui.PushStyleColor(imgui.StyleColorText, style.ColorGold)
	imgui.PushTextWrapPos()
	imgui.TextWrapped(text)
	imgui.PopTextWrapPos()
	imgui.PopStyleColor()
}

func (p *Panel) selectionButton(label string, enabled bool, disabledReason string, action func()) {
	imgui.BeginDisabledV(!enabled)
	if imgui.ButtonV(label, imgui.Vec2{X: -1}) {
		action()
	}
	imgui.EndDisabled()
	if !enabled && imgui.IsItemHoveredV(imgui.HoveredFlagsAllowWhenDisabled) {
		imgui.SetTooltip(disabledReason)
	}
}

// grabSelection is the Grab tool's rectangle on the level being edited.
func (p *Panel) grabSelection() ([]util.Point, bool) {
	grab, ok := tools.Selected().(*tools.ToolGrab)
	if !ok || !grab.HasSelectedArea() {
		return nil, false
	}
	bounds := grab.Bounds()
	var coords []util.Point
	for y := int(bounds.Y1); y <= int(bounds.Y2); y++ {
		for x := int(bounds.X1); x <= int(bounds.X2); x++ {
			coords = append(coords, util.Point{X: x, Y: y, Z: p.editor.ActiveLevel()})
		}
	}
	return coords, true
}

func (p *Panel) selectedTemplate() *automap.Template {
	for _, t := range p.layer.Templates {
		if t.Name == p.sessionAutomap.selected && !t.Hidden() {
			return t
		}
	}
	return nil
}

func (p *Panel) suggestTemplateName() string {
	mapName := p.editor.Dmm().Name
	base := strings.ToLower(strings.TrimSuffix(mapName, filepath.Ext(mapName))) + "_edit"
	name := base
	for n := 2; p.layer.NameTaken(name); n++ {
		name = fmt.Sprintf("%s_%d", base, n)
	}
	return name
}

func (p *Panel) doNewTemplate(coords []util.Point) {
	name := p.sessionAutomap.newName
	log.Printf("do new automap template [%s]: %d tiles", name, len(coords))

	t, added, err := p.layer.NewTemplate(name, p.sessionAutomap.newTrait, p.editor.Dmm().Name, p.editor.ActiveLevel(), coords)
	if err != nil {
		showAutomapError("Can't create the template", err)
		return
	}

	var dropped automap.RemovedTiles
	p.editor.CommitTemplateChange("New Template "+name, func() {
		dropped = p.layer.DropTiles(t, added)
	}, func() {
		p.layer.RestoreTiles(t, dropped)
	})

	p.sessionAutomap.selected = name
	p.sessionAutomap.prefilled = false
}

func (p *Panel) doNewTemplatesFromEdits() {
	baseName := p.sessionAutomap.newName
	log.Printf("do new automap templates from edits [%s]", baseName)

	if !automap.ValidName.MatchString(baseName) {
		showAutomapError("Can't create templates", errors.New("names can only use a-z, 0-9 and _"))
		return
	}

	created, err := p.layer.NewTemplatesFromEdits(p.app.LoadedEnvironment(), p.editor.Dmm(), baseName, p.sessionAutomap.newTrait)
	if len(created) > 0 {
		// one undo step for the lot, even when a later group failed
		dropped := make([]automap.RemovedTiles, len(created))
		p.editor.CommitTemplateChange("New Templates from Edits", func() {
			for idx, t := range created {
				dropped[idx] = p.layer.DropTiles(t, t.Coords())
			}
		}, func() {
			for idx, t := range created {
				p.layer.RestoreTiles(t, dropped[idx])
			}
		})
		p.sessionAutomap.selected = created[0].Name
		p.sessionAutomap.prefilled = false
	}

	if err != nil {
		showAutomapError("Can't create templates", err)
		return
	}
	if len(created) == 0 {
		appdialog.Open(appdialog.TypeInformation{
			Title:       "No edits found",
			Information: "Every tile outside the existing templates matches the map's git HEAD.",
		})
		return
	}

	summary := fmt.Sprintf("Made %d template(s), saved on the next save:\n", len(created))
	for _, t := range created {
		lo, hi := t.Bounds()
		summary += fmt.Sprintf(" - %s: %d tiles, %d,%d to %d,%d on z %d\n", t.Name, t.Size(), lo.X, lo.Y, hi.X, hi.Y, t.Z)
	}
	appdialog.Open(appdialog.TypeInformation{Title: "Templates from edits", Information: summary})
}

func (p *Panel) doAddTiles(t *automap.Template, coords []util.Point) {
	log.Printf("do add tiles to automap template [%s]: %d tiles", t.Name, len(coords))

	added, err := p.layer.AddTiles(t, coords)
	if err != nil {
		showAutomapError("Can't add those tiles", err)
		return
	}
	if len(added) == 0 {
		return
	}

	var dropped automap.RemovedTiles
	p.editor.CommitTemplateChange("Add Tiles to "+t.Name, func() {
		dropped = p.layer.DropTiles(t, added)
	}, func() {
		p.layer.RestoreTiles(t, dropped)
	})
}

func (p *Panel) doRemoveTiles(t *automap.Template, coords []util.Point) {
	owned := 0
	for _, coord := range coords {
		if t.Owns(coord) {
			owned++
		}
	}
	if owned == 0 {
		return
	}

	// emptying a saved template means the next save deletes its file and config entry, so ask first
	if owned == t.Size() && t.OnDisk() {
		appdialog.Open(appdialog.TypeConfirmation{
			Title:    "Delete Template",
			Question: fmt.Sprintf("This removes every tile from %s.\nThe next save deletes %s and its config entry.\nContinue?", t.Name, t.Path()),
			ActionYes: func() {
				p.removeTiles(t, coords)
			},
		})
		return
	}
	p.removeTiles(t, coords)
}

func (p *Panel) removeTiles(t *automap.Template, coords []util.Point) {
	log.Printf("do remove tiles from automap template [%s]: %d tiles", t.Name, len(coords))

	removed, err := p.layer.RemoveTiles(t, coords)
	if err != nil {
		showAutomapError("Can't remove those tiles", err)
		return
	}

	removedCoords := make([]util.Point, 0, len(removed))
	for coord := range removed {
		removedCoords = append(removedCoords, coord)
	}
	p.editor.CommitTemplateChange("Remove Tiles from "+t.Name, func() {
		p.layer.RestoreTiles(t, removed)
	}, func() {
		p.layer.DropTiles(t, removedCoords)
	})
}

func showAutomapError(title string, err error) {
	log.Printf("%s: %v", title, err)
	appdialog.Open(appdialog.TypeInformation{
		Title:       title,
		Information: err.Error(),
	})
}
