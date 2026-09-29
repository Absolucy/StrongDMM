package psettings

import (
	"sdmm/internal/app/config"
	"sdmm/internal/app/window"
	"sdmm/internal/dmapi/automap"
	"sdmm/internal/dmapi/dm"
	"sdmm/internal/dmapi/dmenv"
	"sdmm/internal/dmapi/dmmap"
	"sdmm/internal/util"

	"github.com/SpaiR/imgui-go"
)

type App interface {
	PathsFilter() *dm.PathsFilter
	LoadedEnvironment() *dmenv.Dme

	ConfigRegister(config.Config)
}

type editor interface {
	ActiveLevel() int

	Dmm() *dmmap.Dmm
	CommitMapSizeChange(oldMaxX, oldMaxY, oldMaxZ int)
	CommitTemplateChange(name string, undo, redo func())
	FocusCameraOnPosition(coord util.Point)
}

type Panel struct {
	app App

	editor editor
	layer  *automap.Layer

	sessionMapSize    *sessionMapSize
	sessionScreenshot *sessionScreenshot
	sessionAutomap    sessionAutomap
}

var cfg *psettingsConfig

func New(app App, editor editor, layer *automap.Layer) *Panel {
	if cfg == nil {
		cfg = loadConfig(app)
	}
	return &Panel{app: app, editor: editor, layer: layer, sessionScreenshot: &sessionScreenshot{}}
}

func (p *Panel) Process() {
	imgui.Dummy(imgui.Vec2{X: p.headerSize()})
	p.showMapSize()
	p.showScreenshot()
	p.showAutomap()
}

func (p *Panel) headerSize() float32 {
	return window.PointSize() * 150
}
